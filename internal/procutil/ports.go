package procutil

import (
	"errors"
	"fmt"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"
)

// PortInUseError se devuelve cuando un puerto está ocupado. Trae el PID y el
// nombre del proceso dueño cuando se pudo averiguar (PID 0 = desconocido).
type PortInUseError struct {
	Port        int
	PID         int
	ProcessName string
	Cause       error // error original del bind, si lo hubo
}

func (e *PortInUseError) Error() string {
	switch {
	case e.PID == 4:
		// PID 4 es "System": un componente del kernel. En 80/443 casi siempre es
		// http.sys (IIS, WinRM, etc.); en otros puertos puede ser SMB u otro servicio.
		return fmt.Sprintf("el puerto %d está en uso por Windows (PID 4 \"System\"; en 80/443 suele ser IIS o http.sys)", e.Port)
	case e.PID != 0 && e.ProcessName != "":
		return fmt.Sprintf("el puerto %d está en uso por %s (PID %d)", e.Port, e.ProcessName, e.PID)
	case e.PID != 0:
		return fmt.Sprintf("el puerto %d está en uso por el proceso con PID %d", e.Port, e.PID)
	case e.Cause != nil:
		// Nadie escucha, pero Windows no deja usarlo. Típico: rango de puertos
		// reservado por Hyper-V/WSL (ver `netsh interface ipv4 show excludedportrange protocol=tcp`).
		return fmt.Sprintf("no se puede usar el puerto %d: %v (¿reservado por Hyper-V/WSL?)", e.Port, e.Cause)
	default:
		return fmt.Sprintf("el puerto %d está en uso", e.Port)
	}
}

// CheckPortFree devuelve nil si el puerto TCP está libre, o un *PortInUseError si no.
//
// Hace dos chequeos:
//  1. Busca en la tabla TCP de Windows si algún proceso está escuchando en ese
//     puerto (así podemos decir QUIÉN lo usa).
//  2. Intenta abrir el puerto nosotros mismos (en 0.0.0.0 y 127.0.0.1) y lo
//     cierra al toque. Esto detecta casos que la tabla no muestra, como puertos
//     reservados por Hyper-V.
func CheckPortFree(port int) error {
	if pid, ok := findListenerPID(port); ok {
		return &PortInUseError{Port: port, PID: pid, ProcessName: processName(pid)}
	}

	for _, addr := range []string{"0.0.0.0", "127.0.0.1"} {
		ln, err := net.Listen("tcp4", fmt.Sprintf("%s:%d", addr, port))
		if err != nil {
			return &PortInUseError{Port: port, Cause: err}
		}
		ln.Close()
	}
	return nil
}

// --- Tabla TCP (GetExtendedTcpTable de iphlpapi.dll) ---
//
// x/sys/windows no trae esta función, así que la cargamos de la DLL del sistema.
// NewLazySystemDLL solo busca en System32 (evita cargar una DLL falsa de otra carpeta).

var (
	iphlpapi                = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
)

const (
	afINET  = 2  // IPv4
	afINET6 = 23 // IPv6

	// TCP_TABLE_OWNER_PID_LISTENER: solo sockets en estado LISTEN, con el PID dueño.
	tcpTableOwnerPIDListener = 3
)

// Una fila de la tabla IPv4 (MIB_TCPROW_OWNER_PID).
type tcpRowOwnerPID struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32 // en "network byte order" (big endian) en los 16 bits bajos
	RemoteAddr uint32
	RemotePort uint32
	OwningPID  uint32
}

// Una fila de la tabla IPv6 (MIB_TCP6ROW_OWNER_PID).
type tcp6RowOwnerPID struct {
	LocalAddr     [16]byte
	LocalScopeID  uint32
	LocalPort     uint32
	RemoteAddr    [16]byte
	RemoteScopeID uint32
	RemotePort    uint32
	State         uint32
	OwningPID     uint32
}

// findListenerPID busca un socket escuchando en el puerto (IPv4 o IPv6) y devuelve su PID.
func findListenerPID(port int) (int, bool) {
	// IPv4
	if buf, n, err := getTcpTable(afINET); err == nil {
		rows := unsafe.Slice((*tcpRowOwnerPID)(unsafe.Pointer(&buf[4])), n)
		for _, r := range rows {
			if ntohs(r.LocalPort) == port {
				return int(r.OwningPID), true
			}
		}
	}
	// IPv6
	if buf, n, err := getTcpTable(afINET6); err == nil {
		rows := unsafe.Slice((*tcp6RowOwnerPID)(unsafe.Pointer(&buf[4])), n)
		for _, r := range rows {
			if ntohs(r.LocalPort) == port {
				return int(r.OwningPID), true
			}
		}
	}
	return 0, false
}

// getTcpTable pide la tabla a Windows. La respuesta es un bloque de memoria:
// primero un uint32 con la cantidad de filas, y después las filas seguidas.
// Como no sabemos el tamaño de antemano, se pregunta y se reintenta si no alcanza.
func getTcpTable(family uint32) ([]byte, int, error) {
	size := uint32(16 * 1024)
	for attempt := 0; attempt < 5; attempt++ {
		buf := make([]byte, size)
		ret, _, _ := procGetExtendedTcpTable.Call(
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&size)), // si no alcanza, Windows escribe aquí el tamaño necesario
			0,                              // sin ordenar
			uintptr(family),
			tcpTableOwnerPIDListener,
			0,
		)
		switch windows.Errno(ret) {
		case 0:
			n := int(*(*uint32)(unsafe.Pointer(&buf[0])))
			if n == 0 {
				return nil, 0, errors.New("tabla vacía")
			}
			return buf, n, nil
		case windows.ERROR_INSUFFICIENT_BUFFER:
			continue // size ya trae el tamaño correcto; reintentar
		default:
			return nil, 0, windows.Errno(ret)
		}
	}
	return nil, 0, errors.New("no se pudo leer la tabla TCP")
}

// ntohs convierte el puerto de "network byte order" a número normal.
// Ej: el puerto 80 (0x0050) llega como 0x5000; hay que intercambiar los dos bytes.
func ntohs(p uint32) int {
	lo := p & 0xff        // byte bajo
	hi := (p >> 8) & 0xff // byte alto
	return int(lo<<8 | hi)
}

// processName devuelve el nombre del ejecutable (ej. "httpd.exe") de un PID.
// Se usa una "foto" de procesos (Toolhelp32) en vez de abrir el proceso, porque
// así funciona aunque el proceso sea de otro usuario o corra como admin.
// Devuelve "" si no se encuentra.
func processName(pid int) string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if entry.ProcessID == uint32(pid) {
			return windows.UTF16ToString(entry.ExeFile[:])
		}
	}
	return ""
}
