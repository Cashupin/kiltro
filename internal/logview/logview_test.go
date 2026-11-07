package logview

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kiltro/internal/config"
)

func TestTailUltimasLineas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.log")
	var b strings.Builder
	for i := 1; i <= 1000; i++ {
		fmt.Fprintf(&b, "linea %d\r\n", i) // CRLF como en Windows
	}
	os.WriteFile(path, []byte(b.String()), 0o644)

	r, err := tailFile(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Lines) != 10 || r.Lines[0] != "linea 991" || r.Lines[9] != "linea 1000" || !r.Truncated {
		t.Fatalf("resultado inesperado: %+v", r)
	}
}

func TestTailArchivoGrandeSoloLeeElFinal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grande.log")
	line := strings.Repeat("x", 99) + "\n"                                   // 100 bytes por línea
	os.WriteFile(path, []byte(strings.Repeat(line, 5000)+"ultima\n"), 0o644) // ~500 KB

	r, err := tailFile(path, 100000)
	if err != nil {
		t.Fatal(err)
	}
	if r.Lines[len(r.Lines)-1] != "ultima" || !r.Truncated {
		t.Fatalf("debería terminar en 'ultima' y estar truncado: %d líneas, truncated=%v", len(r.Lines), r.Truncated)
	}
	if len(r.Lines) > maxTailBytes/100+1 {
		t.Fatalf("leyó más de %d bytes: %d líneas", maxTailBytes, len(r.Lines))
	}
	for _, l := range r.Lines[:len(r.Lines)-1] {
		if len(l) != 99 {
			t.Fatalf("la primera línea no debería quedar cortada a la mitad: %q", l)
		}
	}
}

func TestTailSinArchivo(t *testing.T) {
	r, err := tailFile(filepath.Join(t.TempDir(), "no.log"), 10)
	if err != nil || r.Exists || len(r.Lines) != 0 {
		t.Fatalf("sin archivo: %+v %v", r, err)
	}
}

func TestClearYIdsDesconocidos(t *testing.T) {
	root := t.TempDir()
	t.Setenv(config.EnvRoot, root)
	os.MkdirAll(filepath.Join(root, "logs"), 0o755)
	os.WriteFile(filepath.Join(root, "logs", "kiltro.log"), []byte("hola\n"), 0o644)

	if err := Clear("kiltro"); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(filepath.Join(root, "logs", "kiltro.log")); info.Size() != 0 {
		t.Error("el log debería quedar vacío")
	}
	if _, err := Tail("../../Windows/System32/drivers/etc/hosts", 10); err == nil {
		t.Error("un id que no está en la lista debería rechazarse")
	}
	if err := Clear("cualquiera"); err == nil {
		t.Error("un id que no está en la lista debería rechazarse")
	}
}
