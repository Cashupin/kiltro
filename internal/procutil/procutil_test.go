package procutil

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Ocupamos un puerto nosotros mismos y CheckPortFree debe detectarlo,
// diciendo que el dueño es este mismo proceso de test.
func TestCheckPortFreeDetectaPuertoOcupado(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0") // :0 = Windows elige un puerto libre
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	err = CheckPortFree(port)
	var inUse *PortInUseError
	if !errors.As(err, &inUse) {
		t.Fatalf("se esperaba PortInUseError, llegó: %v", err)
	}
	if inUse.PID != os.Getpid() {
		t.Fatalf("PID = %d, se esperaba %d", inUse.PID, os.Getpid())
	}
	exe, _ := os.Executable()
	if !strings.EqualFold(inUse.ProcessName, filepath.Base(exe)) {
		t.Fatalf("nombre = %q, se esperaba %q", inUse.ProcessName, filepath.Base(exe))
	}
	t.Log(err)

	// Al liberarlo, debe quedar libre.
	ln.Close()
	if err := CheckPortFree(port); err != nil {
		t.Fatalf("el puerto debería estar libre: %v", err)
	}
}

// El proceso lanzado con Start debe ejecutarse de verdad (o sea, se reanudó
// después de crearse suspendido): `cmd /c exit 7` tiene que terminar con código 7.
func TestStartReanudaElProceso(t *testing.T) {
	job, err := NewJob()
	if err != nil {
		t.Fatal(err)
	}
	defer job.Close()

	cmd := exec.Command("cmd", "/c", "exit 7")
	if err := Start(job, cmd); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("se esperaba código de salida 7, llegó: %v", err)
	}
}

// Al cerrar el Job, el proceso (y sus hijos) deben morir.
// `cmd /c ping` lanza ping.exe como hijo de cmd.exe: probamos que ambos mueren.
func TestCerrarJobMataLosProcesos(t *testing.T) {
	job, err := NewJob()
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("cmd", "/c", "ping -n 60 127.0.0.1 >nul")
	if err := Start(job, cmd); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond) // darle tiempo a cmd de lanzar ping

	pingPID := childPID(cmd.Process.Pid, "PING.EXE")
	if pingPID == 0 {
		t.Fatal("no se encontró el ping.exe hijo de cmd.exe")
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	job.Close()

	select {
	case <-done:
		// cmd.exe murió, bien
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("el proceso siguió vivo después de cerrar el Job")
	}

	// El nieto (ping.exe) también tiene que haber muerto.
	time.Sleep(200 * time.Millisecond)
	if processName(pingPID) != "" {
		t.Fatalf("ping.exe (PID %d) siguió vivo después de cerrar el Job", pingPID)
	}
}

// childPID busca un hijo directo de parent con ese nombre de exe. 0 si no hay.
func childPID(parent int, exeName string) int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)

	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ParentProcessID == uint32(parent) && strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exeName) {
			return int(e.ProcessID)
		}
	}
	return 0
}
