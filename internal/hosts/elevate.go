package hosts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Cómo se escribe el hosts sin que la app corra como admin:
//
//  1. La app (sin admin) escribe la lista de dominios en un archivo "pendiente"
//     (etc/hosts-pending.txt) y relanza el PROPIO kiltro.exe con
//     `--write-hosts <archivo>` usando el verbo "runas". Eso muestra el aviso de
//     UAC ("¿Permitir que esta app haga cambios?").
//  2. Si el usuario acepta, ese segundo kiltro.exe corre como admin, NO abre
//     ventana (main.go lo detecta antes de arrancar Wails), ejecuta RunElevated
//     y termina. Deja el resultado en <archivo>.result.
//  3. La app espera a que termine y lee el resultado.
//
// El proceso elevado no usa KILTRO_ROOT ni config.AppRoot(): un proceso elevado
// no hereda el entorno de forma confiable. Todo lo que necesita sale de la ruta
// del archivo pendiente que recibe como argumento.

const (
	pendingName   = "hosts-pending.txt"
	backupDirName = "hosts-backup"
	keepBackups   = 10
	elevatedWait  = 2 * time.Minute // tiempo máximo para que el usuario responda el UAC
)

// ErrCancelled se devuelve si el usuario rechazó el aviso de UAC.
var ErrCancelled = errors.New("actualización del hosts cancelada (no se aceptó el permiso de administrador)")

// RequestUpdate pide (con UAC) que el bloque de Kiltro del hosts quede con esos
// dominios. workDir es donde se dejan el archivo pendiente y los backups (etc/).
// Bloquea hasta que el proceso elevado termina.
func RequestUpdate(domains []string, workDir string) error {
	for _, d := range domains {
		if !ValidDomain(d) {
			return fmt.Errorf("dominio inválido para el hosts: %q", d)
		}
	}

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return err
	}
	pending := filepath.Join(workDir, pendingName)
	result := pending + ".result"
	_ = os.Remove(result)

	if err := os.WriteFile(pending, []byte(strings.Join(domains, "\n")+"\n"), 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", pending, err)
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	exitCode, err := runElevatedAndWait(exe, `--write-hosts "`+pending+`"`)
	if err != nil {
		return err
	}

	// El proceso elevado deja "ok" o el mensaje de error.
	msg, readErr := os.ReadFile(result)
	_ = os.Remove(result)
	_ = os.Remove(pending)
	switch {
	case readErr == nil && strings.TrimSpace(string(msg)) == "ok":
		return nil
	case readErr == nil:
		return fmt.Errorf("no se pudo escribir el hosts: %s", strings.TrimSpace(string(msg)))
	default:
		return fmt.Errorf("el proceso elevado terminó (código %d) sin dejar resultado", exitCode)
	}
}

// RunElevated es lo que corre kiltro.exe cuando se lanza con --write-hosts.
// Devuelve el código de salida del proceso.
func RunElevated(pendingFile string) int {
	backupDir := filepath.Join(filepath.Dir(pendingFile), backupDirName)
	err := applyPending(pendingFile, SystemPath(), backupDir)

	msg := "ok"
	code := 0
	if err != nil {
		msg = err.Error()
		code = 1
	}
	_ = os.WriteFile(pendingFile+".result", []byte(msg), 0o644)
	return code
}

// applyPending lee la lista de dominios, hace backup del hosts y reemplaza
// solo el bloque de Kiltro. Recibe las rutas como parámetros para poder
// probarla con archivos de prueba.
func applyPending(pendingFile, hostsPath, backupDir string) error {
	data, err := os.ReadFile(pendingFile)
	if err != nil {
		return fmt.Errorf("no se pudo leer %s: %w", pendingFile, err)
	}
	var domains []string
	for _, line := range strings.Split(string(data), "\n") {
		d := strings.TrimSpace(line)
		if d == "" {
			continue
		}
		// Se valida OTRA VEZ aquí, en el proceso con permisos de admin:
		// es la única validación que de verdad importa.
		if !ValidDomain(d) {
			return fmt.Errorf("dominio inválido, no se escribe nada: %q", d)
		}
		domains = append(domains, d)
	}

	// Releer el hosts actual (puede haber cambiado desde que la app lo leyó).
	raw, err := os.ReadFile(hostsPath)
	if err != nil {
		return fmt.Errorf("no se pudo leer el hosts: %w", err)
	}
	content := string(raw)

	updated, err := ReplaceBlock(content, domains)
	if err != nil {
		return err
	}
	if updated == content {
		return nil // ya estaba al día
	}

	if err := backup(raw, backupDir); err != nil {
		return fmt.Errorf("no se pudo hacer el backup del hosts (no se modificó): %w", err)
	}

	// Se sobrescribe en el lugar (no con "escribir temporal + renombrar") para
	// conservar los permisos (ACL) originales del archivo hosts.
	if err := os.WriteFile(hostsPath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir el hosts: %w", err)
	}
	return nil
}

// backup guarda una copia con fecha y deja solo las últimas keepBackups.
func backup(content []byte, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := "hosts-" + time.Now().Format("20060102-150405")
	if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
		return err
	}

	// Los nombres llevan la fecha en formato ordenable: ordenar = ordenar por fecha.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // el backup ya está hecho; limpiar es secundario
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "hosts-") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keepBackups {
		_ = os.Remove(filepath.Join(dir, names[0]))
		names = names[1:]
	}
	return nil
}

// --- ShellExecuteExW ---
//
// x/sys/windows trae ShellExecute, pero esa versión no devuelve el proceso
// lanzado, así que no se puede esperar a que termine. ShellExecuteExW sí
// (con SEE_MASK_NOCLOSEPROCESS), por eso se llama directo a shell32.dll.

var (
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = shell32.NewProc("ShellExecuteExW")
)

const (
	seeMaskNoCloseProcess = 0x00000040 // devolver el handle del proceso en hProcess
	swHide                = 0
)

// shellExecuteInfo es la estructura SHELLEXECUTEINFOW de Windows. Go la alinea
// igual que C, así que los campos van en el mismo orden y tipo.
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         windows.Handle
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     windows.Handle
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    windows.Handle
	dwHotKey     uint32
	hIcon        windows.Handle // en C es una union hIcon/hMonitor
	hProcess     windows.Handle
}

// runElevatedAndWait lanza exe con "runas" (aviso de UAC), espera a que termine
// y devuelve su código de salida.
func runElevatedAndWait(exe, params string) (uint32, error) {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	args, err := windows.UTF16PtrFromString(params)
	if err != nil {
		return 0, err
	}

	info := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: args,
		nShow:        swHide,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))

	ok, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return 0, ErrCancelled
		}
		return 0, fmt.Errorf("no se pudo pedir permisos de administrador: %w", callErr)
	}
	if info.hProcess == 0 {
		return 0, errors.New("no se obtuvo el proceso elevado")
	}
	defer windows.CloseHandle(info.hProcess)

	ev, err := windows.WaitForSingleObject(info.hProcess, uint32(elevatedWait.Milliseconds()))
	if err != nil {
		return 0, err
	}
	if ev == uint32(windows.WAIT_TIMEOUT) {
		return 0, errors.New("el proceso elevado no terminó a tiempo")
	}

	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return 0, err
	}
	return code, nil
}
