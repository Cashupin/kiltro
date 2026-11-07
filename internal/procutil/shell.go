package procutil

import (
	"golang.org/x/sys/windows"
)

// ShellOpen abre un archivo, carpeta o programa como si el usuario le hiciera
// doble clic (ShellExecute de Windows). params y dir pueden ir vacíos.
//
// Es lo opuesto a Start: el programa abierto NO queda dentro del Job Object
// de Kiltro. Se usa para cosas del usuario (el Explorador, una terminal) que
// tienen que seguir abiertas aunque Kiltro se cierre. Nunca para servicios.
func ShellOpen(file, params, dir string) error {
	var p, d *uint16
	if params != "" {
		p = windows.StringToUTF16Ptr(params)
	}
	if dir != "" {
		d = windows.StringToUTF16Ptr(dir)
	}
	return windows.ShellExecute(0, windows.StringToUTF16Ptr("open"), windows.StringToUTF16Ptr(file), p, d, windows.SW_SHOWNORMAL)
}
