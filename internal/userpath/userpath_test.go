package userpath

import (
	"strings"
	"testing"
)

// Estos tests solo prueban el manejo del texto del PATH; no tocan el registro.

func TestSplitSkipsEmpty(t *testing.T) {
	got := split(`C:\a;;C:\b; ;`)
	if strings.Join(got, "|") != `C:\a|C:\b` {
		t.Fatalf("split = %q", got)
	}
}

func TestSameIgnoresCaseAndTrailingSlash(t *testing.T) {
	if !same(`C:\Kiltro\bin\shims\`, `c:\kiltro\BIN\shims`) {
		t.Fatal("deberían ser la misma carpeta")
	}
	if same(`C:\kiltro\bin\shims`, `C:\kiltro\bin`) {
		t.Fatal("no deberían ser la misma carpeta")
	}
}

func TestWithoutKeepsOthersInOrder(t *testing.T) {
	parts := []string{`C:\kiltro\bin\shims`, `%USERPROFILE%\go\bin`, `C:\KILTRO\bin\shims\`, `C:\x`}
	got := without(parts, `C:\kiltro\bin\shims`)
	if strings.Join(got, ";") != `%USERPROFILE%\go\bin;C:\x` {
		t.Fatalf("without = %q", got)
	}
}
