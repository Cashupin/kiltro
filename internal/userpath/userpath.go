// Package userpath agrega o quita una carpeta del PATH del usuario de Windows.
//
// El PATH del usuario vive en el registro, en HKEY_CURRENT_USER\Environment
// (valor "Path"). Ahí se puede escribir sin permisos de administrador; el PATH
// "del sistema" (HKLM) no se toca nunca.
//
// Ojo: cambiar el registro NO cambia el PATH de los programas que ya están
// abiertos (cada proceso copia su entorno al arrancar). Por eso, después de
// escribir, se avisa a Windows con WM_SETTINGCHANGE: el Explorador relee el
// registro y las terminales que se abran DESDE AHÍ ya ven el cambio. Las que ya
// estaban abiertas no.
package userpath

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const envKey = `Environment`

// Contains dice si dir ya está en el PATH del usuario.
func Contains(dir string) (bool, error) {
	value, _, err := read()
	if err != nil {
		return false, err
	}
	return indexOf(split(value), dir) >= 0, nil
}

// Add pone dir al PRINCIPIO del PATH del usuario (para que gane sobre otros PHP
// instalados, ej. el de Laragon o winget). Si ya estaba, no hace nada.
// Devuelve true si lo agregó.
func Add(dir string) (bool, error) {
	value, typ, err := read()
	if err != nil {
		return false, err
	}
	parts := split(value)
	if indexOf(parts, dir) >= 0 {
		return false, nil
	}
	parts = append([]string{dir}, parts...)
	if err := write(strings.Join(parts, ";"), typ); err != nil {
		return false, err
	}
	// También en el entorno de Kiltro: así la terminal que abre el botón
	// "Terminal" de un sitio ya lo ve, sin reiniciar Kiltro.
	if indexOf(split(os.Getenv("PATH")), dir) < 0 {
		os.Setenv("PATH", dir+";"+os.Getenv("PATH"))
	}
	broadcast()
	return true, nil
}

// Remove quita dir del PATH del usuario (todas las veces que aparezca).
// Devuelve true si estaba.
func Remove(dir string) (bool, error) {
	value, typ, err := read()
	if err != nil {
		return false, err
	}
	parts := split(value)
	kept := without(parts, dir)
	if len(kept) == len(parts) {
		return false, nil
	}
	if err := write(strings.Join(kept, ";"), typ); err != nil {
		return false, err
	}
	os.Setenv("PATH", strings.Join(without(split(os.Getenv("PATH")), dir), ";"))
	broadcast()
	return true, nil
}

// read devuelve el valor de Path tal cual está en el registro (sin expandir
// las %VARIABLES%, para volver a guardarlas igual) y su tipo.
func read() (string, uint32, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, envKey, registry.QUERY_VALUE)
	if err != nil {
		return "", 0, err
	}
	defer k.Close()
	value, typ, err := k.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		return "", registry.EXPAND_SZ, nil // el usuario no tiene PATH propio todavía
	}
	if err != nil {
		return "", 0, err
	}
	return value, typ, nil
}

// write guarda el PATH respetando el tipo que tenía. REG_EXPAND_SZ es el normal:
// permite entradas como %USERPROFILE%\... (si se guardara como REG_SZ, esas
// entradas dejarían de funcionar).
func write(value string, typ uint32) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, envKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if typ == registry.SZ {
		return k.SetStringValue("Path", value)
	}
	return k.SetExpandStringValue("Path", value)
}

func split(value string) []string {
	var parts []string
	for _, p := range strings.Split(value, ";") {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// same compara dos carpetas como lo hace Windows: sin distinguir mayúsculas y
// con o sin "\" al final.
func same(a, b string) bool {
	clean := func(s string) string {
		return strings.TrimRight(filepath.Clean(strings.TrimSpace(s)), `\`)
	}
	return strings.EqualFold(clean(a), clean(b))
}

func indexOf(parts []string, dir string) int {
	for i, p := range parts {
		if same(p, dir) {
			return i
		}
	}
	return -1
}

func without(parts []string, dir string) []string {
	var kept []string
	for _, p := range parts {
		if !same(p, dir) {
			kept = append(kept, p)
		}
	}
	return kept
}

// broadcast avisa a todas las ventanas que cambió el entorno (lo mismo que hace
// la ventana "Variables de entorno" de Windows al aceptar). Espera como mucho
// 2 s por ventana que no responda, para no colgar a Kiltro.
func broadcast() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	var result uintptr
	proc.Call(hwndBroadcast, wmSettingChange, 0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(envKey))),
		smtoAbortIfHung, 2000, uintptr(unsafe.Pointer(&result)))
}
