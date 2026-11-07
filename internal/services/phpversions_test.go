package services

import (
	"os"
	"path/filepath"
	"testing"

	"kiltro/internal/config"
)

// mkphp crea bin/php/<name>/php-cgi.exe (vacío) y, si ts, php8ts.dll.
func mkphp(t *testing.T, root, name string, ts bool) {
	t.Helper()
	dir := filepath.Join(root, "bin", "php", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "php-cgi.exe"), nil, 0o644)
	if ts {
		os.WriteFile(filepath.Join(dir, "php8ts.dll"), nil, 0o644)
	}
}

func TestPHPVersionsOrdenNumericoYSeleccion(t *testing.T) {
	root := t.TempDir()
	t.Setenv(config.EnvRoot, root)

	mkphp(t, root, "php-8.9.0-nts-Win32-vs16-x64", false)
	mkphp(t, root, "php-8.10.1-nts-Win32-vs17-x64", false)
	mkphp(t, root, "php-8.3.33-Win32-vs16-x64", true)
	os.MkdirAll(filepath.Join(root, "bin", "php", "php-sin-cgi"), 0o755) // sin php-cgi.exe: se ignora

	list := PHPVersions()
	if len(list) != 3 {
		t.Fatalf("se esperaban 3 versiones, llegaron %+v", list)
	}
	if list[0].Version != "8.3.33" || list[1].Version != "8.9.0" || list[2].Version != "8.10.1" {
		t.Fatalf("orden incorrecto (8.10 debe ir después de 8.9): %+v", list)
	}
	if list[0].NTS || !list[1].NTS {
		t.Errorf("TS/NTS mal detectado: %+v", list)
	}
	if !list[2].Selected {
		t.Errorf("sin config.json debería elegirse la más nueva: %+v", list)
	}

	// Elegir una versión la guarda en config.json.
	if err := savePHPVersion("php-8.3.33-Win32-vs16-x64"); err != nil {
		t.Fatal(err)
	}
	if v, warn, _ := selectedPHP(listPHPVersions()); v.Version != "8.3.33" || warn != "" {
		t.Fatalf("debería usarse la guardada: %+v %q", v, warn)
	}
	if err := savePHPVersion("php-no-existe"); err == nil {
		t.Error("guardar una versión inexistente debería fallar")
	}

	// Si se borra la carpeta elegida, se usa la más nueva con un aviso.
	os.RemoveAll(filepath.Join(root, "bin", "php", "php-8.3.33-Win32-vs16-x64"))
	if v, warn, _ := selectedPHP(listPHPVersions()); v.Version != "8.10.1" || warn == "" {
		t.Fatalf("debería caer en la más nueva con aviso: %+v %q", v, warn)
	}
}
