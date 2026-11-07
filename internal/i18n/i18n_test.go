package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Todos los idiomas tienen exactamente las mismas claves que español.
func TestMismasClavesEnTodosLosIdiomas(t *testing.T) {
	for _, lang := range Supported {
		for k := range catalogs["es"] {
			if _, ok := catalogs[lang][k]; !ok {
				t.Errorf("%s.json: falta la clave %q", lang, k)
			}
		}
		for k := range catalogs[lang] {
			if _, ok := catalogs["es"][k]; !ok {
				t.Errorf("%s.json: sobra la clave %q (no está en es.json)", lang, k)
			}
		}
	}
}

var placeholderRe = regexp.MustCompile(`\{[a-zA-Z]+\}`)

// Una traducción tiene que usar los mismos marcadores ({domain}, {n}...) que el
// original: si no, el valor no aparece o queda un "{domain}" literal.
func TestMismosMarcadores(t *testing.T) {
	for _, lang := range Supported {
		for k, es := range catalogs["es"] {
			want := placeholders(es)
			got := placeholders(catalogs[lang][k])
			if want != got {
				t.Errorf("%s.json %q: marcadores %s, en es.json %s", lang, k, got, want)
			}
		}
	}
}

func placeholders(s string) string {
	found := placeholderRe.FindAllString(s, -1)
	sort.Strings(found)
	return strings.Join(found, ",")
}

// Cada clave que usa el código (Go con T("...") y Svelte con $t('...') o
// $th('...')) tiene que existir. Así un error de tipeo en una clave falla el test
// en vez de mostrar la clave en pantalla.
func TestLasClavesUsadasExisten(t *testing.T) {
	root := filepath.Join("..", "..")
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\bi18n\.T\("([^"]+)"`),
		regexp.MustCompile(`\$th?\('([^']+)'`),
	}
	checked := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "runtime", "build", "dist", "wailsjs", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		// Estos archivos muestran los patrones en sus comentarios (ejemplos).
		if strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "i18n.ts" {
			return nil
		}
		if ext := filepath.Ext(path); ext != ".go" && ext != ".svelte" && ext != ".ts" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, re := range patterns {
			for _, m := range re.FindAllStringSubmatch(string(data), -1) {
				checked++
				key := m[1]
				// Clave armada en tiempo de ejecución, ej. $t('status.' + s.status):
				// basta con que exista alguna clave con ese prefijo.
				if strings.HasSuffix(key, ".") {
					if !hasPrefix(key) {
						t.Errorf("%s usa claves %q*, pero no hay ninguna en es.json", path, key)
					}
					continue
				}
				if _, ok := catalogs["es"][key]; !ok {
					t.Errorf("%s usa la clave %q, que no está en es.json", path, key)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("no se encontró ningún uso de claves: ¿cambió la forma de llamar a T/$t?")
	}
}

func TestTReemplazaMarcadores(t *testing.T) {
	mu.Lock()
	current = "en"
	mu.Unlock()
	if got := T("top.summary", "running", 2, "total", 4); got != "2 of 4 services running" {
		t.Fatalf("T = %q", got)
	}
	if got := T("no.existe"); got != "no.existe" {
		t.Fatalf("clave inexistente: %q", got)
	}
}

func TestResolve(t *testing.T) {
	if Resolve("es") != "es" || Resolve("en") != "en" {
		t.Fatal("una preferencia válida se usa tal cual")
	}
	if l := Resolve("xx"); !isSupported(l) {
		t.Fatalf("preferencia inválida dio %q", l)
	}
}

func hasPrefix(prefix string) bool {
	for k := range catalogs["es"] {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}
