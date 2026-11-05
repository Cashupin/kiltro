package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Con KILTRO_ROOT definida, AppRoot debe devolver esa ruta (en absoluto).
func TestAppRootUsaVariableDeEntorno(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvRoot, dir)

	got, err := AppRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(dir) {
		t.Fatalf("AppRoot() = %q, se esperaba %q", got, dir)
	}
	if want := filepath.Join(dir, "logs", "kiltro.log"); Path("logs", "kiltro.log") != want {
		t.Fatalf("Path() = %q, se esperaba %q", Path("logs", "kiltro.log"), want)
	}
}

// Sin KILTRO_ROOT, AppRoot debe ser la carpeta del ejecutable.
func TestAppRootSinVariableUsaCarpetaDelExe(t *testing.T) {
	t.Setenv(EnvRoot, "")

	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)

	got, err := AppRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Dir(exe) {
		t.Fatalf("AppRoot() = %q, se esperaba %q", got, filepath.Dir(exe))
	}
}
