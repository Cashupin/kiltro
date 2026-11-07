package procutil

import (
	"errors"
	"kiltro/internal/i18n"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ErrCancelled se devuelve si el usuario rechazó el aviso de UAC.
// Es un tipo (no errors.New) para que el texto salga en el idioma en uso al
// mostrarlo, aunque el idioma cambie después de arrancar.
var ErrCancelled error = cancelledError{}

type cancelledError struct{}

func (cancelledError) Error() string { return i18n.T("elevate.cancelled") }

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

// RunElevatedAndWait relanza exe como administrador ("runas": aparece el aviso
// de UAC), espera hasta wait a que termine y devuelve su código de salida.
func RunElevatedAndWait(exe, params string, wait time.Duration) (uint32, error) {
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
		return 0, i18n.Wrap(callErr, "elevate.requestFailed")
	}
	if info.hProcess == 0 {
		return 0, i18n.Err("elevate.noProcess")
	}
	defer windows.CloseHandle(info.hProcess)

	ev, err := windows.WaitForSingleObject(info.hProcess, uint32(wait.Milliseconds()))
	if err != nil {
		return 0, err
	}
	if ev == uint32(windows.WAIT_TIMEOUT) {
		return 0, i18n.Err("elevate.timeout")
	}

	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return 0, err
	}
	return code, nil
}
