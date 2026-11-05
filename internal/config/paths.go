// Package config centraliza la resolución de rutas de la app (y más adelante
// la lectura/escritura de config.json).
//
// Regla del proyecto: NINGÚN otro paquete calcula rutas por su cuenta.
// Todo lo que necesite una ruta (bin/, etc/, www/, data/, logs/...) la pide
// a través de AppRoot() o Path().
package config

import (
	"os"
	"path/filepath"
)

// EnvRoot es la variable de entorno que, si está definida, fuerza la raíz de la app.
// Se usa en desarrollo (`wails dev`), porque ahí el ejecutable se compila en una
// carpeta temporal/build y no junto a bin/, www/, etc.
const EnvRoot = "KILTRO_ROOT"

// AppRoot devuelve la carpeta raíz de la app, desde donde se resuelven todas
// las demás rutas. Orden de prioridad:
//
//  1. Si existe la variable de entorno KILTRO_ROOT (y no está vacía), se usa esa.
//  2. Si no, la carpeta donde está el ejecutable (kiltro.exe).
//
// La ruta devuelta siempre es absoluta y limpia (sin "..", sin separadores dobles).
func AppRoot() (string, error) {
	if root := os.Getenv(EnvRoot); root != "" {
		// Convertimos a absoluta por si alguien puso una ruta relativa
		// (ej. KILTRO_ROOT=runtime). Se resuelve contra el directorio actual.
		return filepath.Abs(root)
	}

	// os.Executable devuelve la ruta completa del .exe que se está ejecutando.
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}

	// Si el .exe se lanzó a través de un symlink/junction, resolvemos la ruta real
	// para que la raíz sea la carpeta donde vive el binario de verdad.
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}

	return filepath.Dir(exe), nil
}

// Path arma una ruta relativa a la raíz de la app, al estilo de base_path() en Laravel.
//
//	config.Path()                     -> C:\kiltro
//	config.Path("logs", "kiltro.log") -> C:\kiltro\logs\kiltro.log
//
// Si AppRoot() falla (algo muy raro: Windows no pudo informar la ruta del .exe),
// usa el directorio actual como respaldo para no tener que propagar el error
// en cada llamada.
func Path(parts ...string) string {
	root, err := AppRoot()
	if err != nil {
		root = "."
		if wd, wdErr := os.Getwd(); wdErr == nil {
			root = wd
		}
	}
	return filepath.Join(append([]string{root}, parts...)...)
}
