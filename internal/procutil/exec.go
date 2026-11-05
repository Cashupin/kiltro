package procutil

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Start lanza cmd sin ventana de consola y lo mete en el Job antes de que
// llegue a ejecutar una sola instrucción.
//
// Por qué se lanza "suspendido":
// si arrancáramos el proceso normal y después lo metiéramos al Job, habría un
// instante en que el proceso ya corre fuera del Job. Si en ese instante lanza
// un hijo propio, ese hijo quedaría fuera del Job y no moriría con Kiltro.
// Para evitarlo:
//  1. Se crea el proceso con CREATE_SUSPENDED (existe, pero su hilo principal está pausado).
//  2. Se asigna al Job.
//  3. Se reanuda su hilo principal.
//
// Después de Start, el llamador usa cmd.Wait() / cmd.Process como siempre.
func Start(job *Job, cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// HideWindow: si el programa intenta mostrar una ventana, que arranque oculta.
	// CREATE_NO_WINDOW: a los programas de consola (nginx, php-cgi, mysqld) no se
	// les crea una ventana de consola negra.
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW | windows.CREATE_SUSPENDED

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("no se pudo iniciar %s: %w", cmd.Path, err)
	}
	pid := cmd.Process.Pid

	if err := job.Assign(pid); err != nil {
		// No lo dejamos corriendo fuera del Job: lo matamos (nunca llegó a ejecutarse).
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}

	if err := resumeProcess(pid); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("no se pudo reanudar %s (PID %d): %w", cmd.Path, pid, err)
	}
	return nil
}

// resumeProcess reanuda los hilos del proceso pid que fue creado con CREATE_SUSPENDED.
//
// Go (os/exec) no nos entrega el handle del hilo principal, así que lo buscamos:
// pedimos a Windows una "foto" de todos los hilos del sistema (Toolhelp32 snapshot)
// y reanudamos los que pertenecen a nuestro PID. Un proceso recién creado y
// suspendido tiene un único hilo.
func resumeProcess(pid int) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snap)

	var entry windows.ThreadEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	resumed := 0
	for err = windows.Thread32First(snap, &entry); err == nil; err = windows.Thread32Next(snap, &entry) {
		if entry.OwnerProcessID != uint32(pid) {
			continue
		}
		th, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if openErr != nil {
			return openErr
		}
		_, resumeErr := windows.ResumeThread(th)
		windows.CloseHandle(th)
		if resumeErr != nil {
			return resumeErr
		}
		resumed++
	}

	if resumed == 0 {
		return fmt.Errorf("no se encontró ningún hilo del proceso")
	}
	return nil
}
