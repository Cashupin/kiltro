package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

// Settings es el contenido de config.json (junto al .exe). Por ahora solo
// guarda la versión de PHP elegida; la etapa 8 le agrega puertos, carpeta www, etc.
//
// Un campo vacío significa "usar el valor por defecto".
type Settings struct {
	// PHP es el nombre de la carpeta dentro de bin/php/ (ej. "php-8.4.27-1-nts-Win32-vs17-x64").
	PHP string `json:"php,omitempty"`

	// SSL: true si el usuario activó HTTPS (la CA de mkcert ya está instalada).
	SSL bool `json:"ssl,omitempty"`
}

// mu evita que dos goroutines escriban config.json a la vez.
var mu sync.Mutex

// Load lee config.json. Si no existe, devuelve la configuración por defecto
// (vacía) sin error: la primera vez que se abre la app todavía no hay archivo.
func Load() (Settings, error) {
	mu.Lock()
	defer mu.Unlock()
	return load()
}

func load() (Settings, error) {
	var s Settings
	data, err := os.ReadFile(Path("config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("no se pudo leer config.json: %w", err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("config.json no es un JSON válido: %w", err)
	}
	return s, nil
}

// Update lee config.json, aplica el cambio y lo guarda. Así se conservan los
// demás campos (al estilo de "leer, modificar, guardar"):
//
//	config.Update(func(s *config.Settings) { s.PHP = "php-8.4..." })
func Update(change func(s *Settings)) error {
	mu.Lock()
	defer mu.Unlock()

	s, err := load()
	if err != nil {
		return err
	}
	change(&s)

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	// Se escribe a un archivo temporal y luego se renombra: si la app se corta
	// justo en medio, config.json no queda a medio escribir.
	tmp := Path("config.json.tmp")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("no se pudo guardar config.json: %w", err)
	}
	if err := os.Rename(tmp, Path("config.json")); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("no se pudo guardar config.json: %w", err)
	}
	return nil
}
