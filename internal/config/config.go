package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Valores por defecto (los que se usan si config.json no dice otra cosa).
const (
	DefaultHTTPPort    = 80
	DefaultHTTPSPort   = 443
	DefaultMySQLPort   = 3306
	DefaultRedisPort   = 6379
	DefaultPHPBasePort = 9000
	DefaultPHPWorkers  = 4
	MaxPHPWorkers      = 16
)

// Settings es el contenido de config.json (junto al .exe).
//
// Los puertos en 0 y la carpeta www vacía significan "usar el valor por
// defecto": Load los completa, así el resto del código no tiene que pensar en eso.
type Settings struct {
	// PHP es el nombre de la carpeta dentro de bin/php/ (ej. "php-8.4.27-1-nts-Win32-vs17-x64").
	PHP string `json:"php,omitempty"`

	// SSL: true si el usuario activó HTTPS (la CA de mkcert ya está instalada).
	SSL bool `json:"ssl,omitempty"`

	HTTPPort  int `json:"httpPort"`  // Nginx http (80)
	HTTPSPort int `json:"httpsPort"` // Nginx https (443), solo si SSL
	MySQLPort int `json:"mysqlPort"` // 3306
	RedisPort int `json:"redisPort"` // 6379

	// PHP: cantidad de workers php-cgi, en puertos consecutivos desde PHPBasePort.
	PHPBasePort int `json:"phpBasePort"`
	PHPWorkers  int `json:"phpWorkers"`

	// LAN: true = Nginx atiende también a otros equipos de la red local (escucha
	// en todas las interfaces). false (por defecto) = solo esta PC (127.0.0.1).
	LAN bool `json:"lan,omitempty"`

	// WWW: carpeta de proyectos. Vacía = <carpeta de la app>/www.
	WWW string `json:"www,omitempty"`
}

// mu evita que dos goroutines escriban config.json a la vez.
var mu sync.Mutex

// Load lee config.json. Si no existe, devuelve la configuración por defecto
// sin error: la primera vez que se abre la app todavía no hay archivo.
func Load() (Settings, error) {
	mu.Lock()
	defer mu.Unlock()
	return load()
}

// Current es Load sin error: si config.json está roto, devuelve los valores
// por defecto. Para los servicios, que necesitan un puerto sí o sí. (El error
// igual se ve al abrir la pantalla de configuración.)
func Current() Settings {
	s, _ := Load()
	return s
}

func load() (Settings, error) {
	var s Settings
	data, err := os.ReadFile(Path("config.json"))
	if errors.Is(err, os.ErrNotExist) {
		s.applyDefaults()
		return s, nil
	}
	if err != nil {
		s.applyDefaults()
		return s, fmt.Errorf("no se pudo leer config.json: %w", err)
	}
	if err := json.Unmarshal(data, &s); err != nil {
		s = Settings{}
		s.applyDefaults()
		return s, fmt.Errorf("config.json no es un JSON válido: %w", err)
	}
	s.applyDefaults()
	return s, nil
}

func (s *Settings) applyDefaults() {
	if s.HTTPPort == 0 {
		s.HTTPPort = DefaultHTTPPort
	}
	if s.HTTPSPort == 0 {
		s.HTTPSPort = DefaultHTTPSPort
	}
	if s.MySQLPort == 0 {
		s.MySQLPort = DefaultMySQLPort
	}
	if s.RedisPort == 0 {
		s.RedisPort = DefaultRedisPort
	}
	if s.PHPBasePort == 0 {
		s.PHPBasePort = DefaultPHPBasePort
	}
	if s.PHPWorkers == 0 {
		s.PHPWorkers = DefaultPHPWorkers
	}
}

// PHPPorts son los puertos de los workers (ej. 9000, 9001, 9002, 9003).
func (s Settings) PHPPorts() []int {
	ports := make([]int, s.PHPWorkers)
	for i := range ports {
		ports[i] = s.PHPBasePort + i
	}
	return ports
}

// WWWDir es la carpeta de proyectos (absoluta).
func (s Settings) WWWDir() string {
	if s.WWW == "" {
		return Path("www")
	}
	return s.WWW
}

// Validate revisa que la configuración tenga sentido antes de guardarla.
func (s Settings) Validate() error {
	named := []struct {
		name string
		port int
	}{
		{"Nginx HTTP", s.HTTPPort},
		{"Nginx HTTPS", s.HTTPSPort},
		{"MySQL", s.MySQLPort},
		{"Redis", s.RedisPort},
	}
	if s.PHPWorkers < 1 || s.PHPWorkers > MaxPHPWorkers {
		return fmt.Errorf("la cantidad de workers de PHP debe estar entre 1 y %d", MaxPHPWorkers)
	}
	for i, p := range s.PHPPorts() {
		named = append(named, struct {
			name string
			port int
		}{fmt.Sprintf("PHP (worker %d)", i+1), p})
	}

	used := map[int]string{}
	for _, n := range named {
		if n.port < 1 || n.port > 65535 {
			return fmt.Errorf("el puerto de %s (%d) debe estar entre 1 y 65535", n.name, n.port)
		}
		if other, ok := used[n.port]; ok {
			return fmt.Errorf("el puerto %d está repetido: lo usan %s y %s", n.port, other, n.name)
		}
		used[n.port] = n.name
	}

	if s.WWW != "" {
		if !filepath.IsAbs(s.WWW) {
			return fmt.Errorf("la carpeta de proyectos debe ser una ruta completa (ej. D:\\proyectos)")
		}
		info, err := os.Stat(s.WWW)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("la carpeta de proyectos no existe: %s", s.WWW)
		}
	}
	return nil
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
	s.applyDefaults()

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
