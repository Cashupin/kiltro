// Package logview lee y vacía los archivos de log de Kiltro para la pestaña
// "Logs" de la UI.
//
// Seguridad: la UI nunca manda una ruta, solo el id de un log de esta lista
// fija. Así no se puede usar este código para leer o vaciar otro archivo.
package logview

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"kiltro/internal/config"
)

// Log es un archivo de log que se puede ver.
type Log struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Path  string `json:"path"`
}

// List devuelve los logs que se pueden ver, en el orden del desplegable.
// Los errores de PHP aparecen en el log de errores de Nginx (llegan por FastCGI).
func List() []Log {
	return []Log{
		{"kiltro", "Kiltro", config.Path("logs", "kiltro.log")},
		{"nginx-error", "Nginx: errores (y errores de PHP)", config.Path("logs", "nginx", "error.log")},
		{"nginx-access", "Nginx: accesos", config.Path("logs", "nginx", "access.log")},
		{"mysql", "MySQL: errores", config.Path("logs", "mysql", "error.log")},
	}
}

func find(id string) (Log, error) {
	for _, l := range List() {
		if l.ID == id {
			return l, nil
		}
	}
	return Log{}, fmt.Errorf("log desconocido: %q", id)
}

// Result es lo que recibe la UI.
type Result struct {
	Lines     []string `json:"lines"`
	Size      int64    `json:"size"`      // tamaño total del archivo en bytes
	Exists    bool     `json:"exists"`    // false si el archivo todavía no se creó
	Truncated bool     `json:"truncated"` // true si hay más líneas antes de las mostradas
}

// maxTailBytes: se leen como mucho los últimos 256 KB del archivo, así un log
// gigante (access.log crece rápido) no se carga entero en memoria.
const maxTailBytes = 256 * 1024

// Tail devuelve las últimas maxLines líneas del log.
func Tail(id string, maxLines int) (Result, error) {
	l, err := find(id)
	if err != nil {
		return Result{}, err
	}
	return tailFile(l.Path, maxLines)
}

func tailFile(path string, maxLines int) (Result, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Result{Lines: []string{}}, nil
	}
	if err != nil {
		return Result{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return Result{}, err
	}
	res := Result{Size: info.Size(), Exists: true, Lines: []string{}}

	// Leer desde (final - 256 KB) hasta el final.
	start := info.Size() - maxTailBytes
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return res, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return res, err
	}
	if start > 0 {
		// Empezamos en medio de una línea: descartarla hasta el primer salto.
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
		res.Truncated = true
	}

	text := strings.TrimRight(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if text == "" {
		return res, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
		res.Truncated = true
	}
	res.Lines = lines
	return res, nil
}

// Clear vacía el log (lo deja en 0 bytes, sin borrarlo).
//
// Se puede hacer aunque el servicio esté corriendo: Kiltro, Nginx y MySQL
// abren sus logs en modo "agregar al final", así que después de vaciarlo
// siguen escribiendo desde el principio sin problema.
func Clear(id string) error {
	l, err := find(id)
	if err != nil {
		return err
	}
	err = os.Truncate(l.Path, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil // no hay nada que vaciar
	}
	if err != nil {
		return fmt.Errorf("no se pudo vaciar %s: %w", l.Path, err)
	}
	return nil
}
