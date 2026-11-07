package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSinArchivoDevuelveVacio(t *testing.T) {
	t.Setenv(EnvRoot, t.TempDir())
	s, err := Load()
	if err != nil || s.PHP != "" {
		t.Fatalf("sin config.json debería devolver vacío sin error: %+v %v", s, err)
	}
}

func TestUpdateGuardaYConservaCampos(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvRoot, dir)

	if err := Update(func(s *Settings) { s.PHP = "php-8.4.27" }); err != nil {
		t.Fatal(err)
	}
	s, err := Load()
	if err != nil || s.PHP != "php-8.4.27" {
		t.Fatalf("no se guardó la versión: %+v %v", s, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json.tmp")); !os.IsNotExist(err) {
		t.Error("no debería quedar config.json.tmp")
	}

	// Un campo desconocido (ej. de una versión más nueva de Kiltro) no rompe la lectura.
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"php":"x","otro":1}`), 0o644)
	if s, err := Load(); err != nil || s.PHP != "x" {
		t.Fatalf("debería ignorar campos desconocidos: %+v %v", s, err)
	}

	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{roto`), 0o644)
	if _, err := Load(); err == nil {
		t.Error("un JSON roto debería dar error")
	}
}

func TestDefaultsYValidate(t *testing.T) {
	t.Setenv(EnvRoot, t.TempDir())
	s, _ := Load()
	if s.HTTPPort != 80 || s.HTTPSPort != 443 || s.MySQLPort != 3306 || s.PHPBasePort != 9000 || s.PHPWorkers != 4 {
		t.Fatalf("defaults incorrectos: %+v", s)
	}
	if got := s.PHPPorts(); len(got) != 4 || got[3] != 9003 {
		t.Fatalf("PHPPorts = %v", got)
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("los defaults deberían ser válidos: %v", err)
	}

	bad := s
	bad.MySQLPort = 9002 // choca con un worker de PHP
	if err := bad.Validate(); err == nil {
		t.Error("debería detectar el puerto repetido")
	}
	bad = s
	bad.HTTPPort = 70000
	if err := bad.Validate(); err == nil {
		t.Error("debería rechazar un puerto > 65535")
	}
	bad = s
	bad.PHPWorkers = 0
	if err := bad.Validate(); err == nil {
		t.Error("debería rechazar 0 workers")
	}
	bad = s
	bad.WWW = filepath.Join(t.TempDir(), "no-existe")
	if err := bad.Validate(); err == nil {
		t.Error("debería rechazar una carpeta inexistente")
	}
	bad.WWW = "relativa"
	if err := bad.Validate(); err == nil {
		t.Error("debería rechazar una ruta relativa")
	}
}
