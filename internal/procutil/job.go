// Package procutil agrupa utilidades de bajo nivel para manejar procesos en Windows:
// Job Objects, lanzar procesos sin ventana y chequeo de puertos.
package procutil

import (
	"kiltro/internal/i18n"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Job envuelve un "Job Object" de Windows.
//
// ¿Qué es un Job Object? Es un contenedor del kernel donde se pueden meter procesos.
// Lo configuramos con JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, que significa:
// "cuando se cierre el último handle a este Job, matar todos los procesos que contiene".
//
// El único handle lo tiene Kiltro. Si Kiltro termina por cualquier motivo
// (se cierra normal, crashea, lo matan desde el Administrador de tareas),
// Windows cierra sus handles automáticamente → el Job se cierra → nginx, php-cgi,
// mysqld, etc. mueren con él. Así no quedan procesos zombie ocupando puertos.
//
// Además, los procesos que lance un proceso que ya está en el Job (nietos)
// también quedan dentro del Job automáticamente.
type Job struct {
	handle windows.Handle
}

// NewJob crea un Job Object nuevo con la opción "matar todo al cerrar".
func NewJob() (*Job, error) {
	// nil, nil = sin atributos de seguridad especiales y sin nombre (Job anónimo).
	// El handle NO es heredable, así que los hijos no reciben una copia:
	// solo Kiltro lo mantiene vivo.
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, i18n.Wrap(err, "job.create")
	}

	// Configurar el límite KILL_ON_JOB_CLOSE. Windows exige pasarlo dentro de
	// la estructura "extended limit information".
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE

	_, err = windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		windows.CloseHandle(h)
		return nil, i18n.Wrap(err, "job.config")
	}

	return &Job{handle: h}, nil
}

// Assign mete en el Job al proceso con ese PID.
func (j *Job) Assign(pid int) error {
	// Para asignar un proceso a un Job, Windows pide abrirlo con estos dos permisos.
	const access = windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE

	p, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return i18n.Wrap(err, "job.open", "pid", pid)
	}
	defer windows.CloseHandle(p)

	if err := windows.AssignProcessToJobObject(j.handle, p); err != nil {
		return i18n.Wrap(err, "job.assign", "pid", pid)
	}
	return nil
}

// Close cierra el handle del Job. Por KILL_ON_JOB_CLOSE, esto MATA a todos los
// procesos que sigan dentro. Normalmente no hace falta llamarlo: los servicios se
// detienen de forma limpia antes, y si Kiltro muere, Windows lo cierra solo.
func (j *Job) Close() error {
	if j.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(j.handle)
	j.handle = 0
	return err
}
