package procutil

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// KillTree termina a la fuerza el proceso pid y todos sus descendientes
// (hijos, nietos...). Es el ÚLTIMO RECURSO, cuando el apagado limpio falló.
//
// Por qué hace falta: en Windows matar un proceso NO mata a sus hijos.
// Ej: nginx tiene un proceso "master" y uno o más "workers"; si solo matamos el
// master, los workers pueden quedar vivos ocupando el puerto 80.
//
// Los descendientes se buscan por "proceso padre" en una foto de procesos.
// Primero se matan los hijos y al final el proceso pedido.
func KillTree(pid int) {
	for _, p := range descendants(pid) {
		terminate(p)
	}
	terminate(pid)
}

// descendants devuelve los PIDs de todos los descendientes de pid
// (los más profundos primero).
func descendants(pid int) []int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)

	// Mapa padre -> hijos.
	children := map[uint32][]uint32{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ProcessID != e.ParentProcessID { // PID 0 es su propio padre; evitar ciclos
			children[e.ParentProcessID] = append(children[e.ParentProcessID], e.ProcessID)
		}
	}

	var out []int
	var walk func(p uint32)
	walk = func(p uint32) {
		for _, c := range children[p] {
			walk(c) // primero los nietos
			out = append(out, int(c))
		}
	}
	walk(uint32(pid))
	return out
}

func terminate(pid int) {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return // ya no existe o no tenemos permiso
	}
	_ = windows.TerminateProcess(h, 1)
	windows.CloseHandle(h)
}
