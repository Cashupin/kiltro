package hosts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const original = "# Copyright (c) Microsoft\r\n#\r\n127.0.0.1 localhost\r\n10.0.0.5 servidor-empresa\r\n"

func TestReplaceBlockAgregaAlFinalSinTocarLoDemas(t *testing.T) {
	got, err := ReplaceBlock(original, []string{"b.test", "a.test"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, original) {
		t.Fatalf("lo de antes del bloque cambió:\n%q", got)
	}
	want := original +
		"# KILTRO START\r\n" +
		"# Administrado por Kiltro: no editar a mano entre estas marcas.\r\n" +
		"127.0.0.1 a.test\r\n" +
		"127.0.0.1 b.test\r\n" +
		"# KILTRO END\r\n"
	if got != want {
		t.Fatalf("resultado inesperado:\n%q\nse esperaba:\n%q", got, want)
	}
}

func TestReplaceBlockReemplazaEnSuLugar(t *testing.T) {
	content := "antes\r\n# KILTRO START\r\n127.0.0.1 viejo.test\r\n# KILTRO END\r\ndespues\r\n"
	got, err := ReplaceBlock(content, []string{"nuevo.test"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "antes\r\n# KILTRO START\r\n") || !strings.HasSuffix(got, "# KILTRO END\r\ndespues\r\n") {
		t.Fatalf("no se respetó lo de fuera del bloque:\n%q", got)
	}
	if strings.Contains(got, "viejo.test") || !strings.Contains(got, "127.0.0.1 nuevo.test") {
		t.Fatalf("el bloque no se reemplazó:\n%q", got)
	}
	if got2, _ := ReplaceBlock(got, []string{"nuevo.test"}); got2 != got {
		t.Fatal("aplicar dos veces lo mismo debería dar el mismo resultado")
	}
}

func TestReplaceBlockSinDominiosQuitaElBloque(t *testing.T) {
	content := "antes\n# KILTRO START\n127.0.0.1 x.test\n# KILTRO END\ndespues\n"
	got, err := ReplaceBlock(content, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "antes\ndespues\n" { // también respeta LF si el archivo usa LF
		t.Fatalf("se esperaba quitar el bloque, quedó:\n%q", got)
	}
	if got, _ := ReplaceBlock(original, nil); got != original {
		t.Fatal("sin bloque y sin dominios no debería tocarse nada")
	}
}

func TestReplaceBlockArchivoSinSaltoFinal(t *testing.T) {
	got, err := ReplaceBlock("127.0.0.1 localhost", []string{"a.test"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "127.0.0.1 localhost\r\n# KILTRO START") {
		t.Fatalf("debería agregar un salto antes del bloque:\n%q", got)
	}
}

func TestReplaceBlockRechazaBloqueRotoYDominiosInvalidos(t *testing.T) {
	if _, err := ReplaceBlock("# KILTRO START\n127.0.0.1 a.test\n", []string{"a.test"}); err == nil {
		t.Error("un START sin END debería ser error (no adivinar)")
	}
	for _, bad := range []string{"banco.com", "a.test evil.com", "A.test", "-a.test", "a.dev", ""} {
		if _, err := ReplaceBlock(original, []string{bad}); err == nil {
			t.Errorf("debería rechazar %q", bad)
		}
	}
}

func TestBlockDomainsYUpToDate(t *testing.T) {
	content, _ := ReplaceBlock(original, []string{"b.test", "a.test"})
	if got := BlockDomains(content); strings.Join(got, ",") != "a.test,b.test" {
		t.Fatalf("BlockDomains = %v", got)
	}
	if !UpToDate(content, []string{"b.test", "a.test"}) {
		t.Error("debería estar al día (el orden no importa)")
	}
	if UpToDate(content, []string{"a.test"}) {
		t.Error("no debería estar al día (sobra b.test)")
	}
	if !UpToDate(original, nil) {
		t.Error("sin sitios y sin bloque está al día")
	}
}

// applyPending es lo que corre el proceso elevado: se prueba con un hosts falso.
func TestApplyPending(t *testing.T) {
	dir := t.TempDir()
	hostsPath := filepath.Join(dir, "hosts")
	pending := filepath.Join(dir, "hosts-pending.txt")
	backups := filepath.Join(dir, "hosts-backup")
	os.WriteFile(hostsPath, []byte(original), 0o644)

	os.WriteFile(pending, []byte("demo.test\nblog.test\n"), 0o644)
	if err := applyPending(pending, hostsPath, backups); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(hostsPath)
	if !UpToDate(string(data), []string{"demo.test", "blog.test"}) {
		t.Fatalf("el hosts no quedó con los dominios:\n%s", data)
	}
	if entries, _ := os.ReadDir(backups); len(entries) != 1 {
		t.Fatalf("se esperaba 1 backup, hay %d", len(entries))
	}

	// Un dominio inválido en el archivo pendiente: no se escribe NADA.
	before, _ := os.ReadFile(hostsPath)
	os.WriteFile(pending, []byte("demo.test\nbanco.com\n"), 0o644)
	if err := applyPending(pending, hostsPath, backups); err == nil {
		t.Fatal("debería rechazar banco.com")
	}
	after, _ := os.ReadFile(hostsPath)
	if string(before) != string(after) {
		t.Fatal("con un dominio inválido el hosts no debe cambiar")
	}
}

func TestBackupDejaSoloLosUltimos(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < keepBackups+5; i++ {
		// nombres ordenables viejos
		os.WriteFile(filepath.Join(dir, "hosts-2020010"+string(rune('a'+i))), []byte("x"), 0o644)
	}
	if err := backup([]byte("nuevo"), dir); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != keepBackups {
		t.Fatalf("se esperaban %d backups, hay %d", keepBackups, len(entries))
	}
}
