// Package i18n tiene los textos de Kiltro en cada idioma (español e inglés).
//
// Los textos están en locales/es.json y locales/en.json (embebidos en el exe).
// Son un solo catálogo para todo Kiltro: los usa Go con T(...) y la interfaz
// (Svelte) los recibe enteros con Messages(...) y los usa con $t(...).
//
// Cada texto tiene una clave, ej. "services.startAll". Los valores pueden tener
// marcadores con nombre entre llaves, que se reemplazan al usarlos:
//
//	"top.summary": "{running} de {total} servicios corriendo"
//	i18n.T("top.summary", "running", 2, "total", 4)  →  "2 de 4 servicios corriendo"
//
// Para agregar un idioma: copiar es.json a <código>.json, traducir los valores
// (las claves no se tocan) y agregarlo a Supported.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

//go:embed locales/*.json
var files embed.FS

// Supported son los idiomas disponibles, en el orden del selector.
var Supported = []string{"es", "en"}

// Fallback es el idioma que se usa si Windows está en un idioma que Kiltro no tiene.
const Fallback = "en"

var (
	mu       sync.RWMutex
	catalogs = map[string]map[string]string{}
	current  = "es" // idioma en uso (lo cambia SetPreference)
)

func init() {
	for _, lang := range Supported {
		data, err := files.ReadFile("locales/" + lang + ".json")
		if err != nil {
			panic(err) // no puede pasar: los archivos van dentro del exe
		}
		m := map[string]string{}
		if err := json.Unmarshal(data, &m); err != nil {
			panic(fmt.Sprintf("locales/%s.json no es JSON válido: %v", lang, err))
		}
		catalogs[lang] = m
	}
	current = Resolve("")
}

// Resolve convierte la preferencia guardada en config.json ("" = automático,
// "es", "en") en el idioma que se va a usar.
func Resolve(pref string) string {
	if isSupported(pref) {
		return pref
	}
	return windowsLang()
}

// SetPreference cambia el idioma en uso. pref = "" (automático), "es" o "en".
// Devuelve el idioma resultante.
func SetPreference(pref string) string {
	lang := Resolve(pref)
	mu.Lock()
	current = lang
	mu.Unlock()
	return lang
}

// Lang es el idioma en uso ("es" o "en").
func Lang() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Messages devuelve todos los textos de un idioma (para la interfaz). Si a ese
// idioma le falta alguna clave, se completa con la de español.
func Messages(lang string) map[string]string {
	out := map[string]string{}
	for k, v := range catalogs["es"] {
		out[k] = v
	}
	for k, v := range catalogs[lang] {
		out[k] = v
	}
	return out
}

// T devuelve el texto de la clave en el idioma en uso, con los marcadores
// reemplazados. args van de a pares: nombre, valor, nombre, valor...
// Si la clave no existe en ningún idioma, devuelve la clave (así se nota).
func T(key string, args ...any) string {
	lang := Lang()
	text, ok := catalogs[lang][key]
	if !ok {
		if text, ok = catalogs["es"][key]; !ok {
			text = key
		}
	}
	for i := 0; i+1 < len(args); i += 2 {
		name := fmt.Sprint(args[i])
		text = strings.ReplaceAll(text, "{"+name+"}", fmt.Sprint(args[i+1]))
	}
	return text
}

func isSupported(lang string) bool {
	for _, l := range Supported {
		if l == lang {
			return true
		}
	}
	return false
}

// windowsLang devuelve el idioma de la interfaz de Windows si Kiltro lo tiene
// (ej. "es-CL" → "es"); si no, Fallback (inglés).
func windowsLang() string {
	langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err == nil && len(langs) > 0 {
		code := strings.ToLower(strings.SplitN(langs[0], "-", 2)[0])
		if isSupported(code) {
			return code
		}
	}
	return Fallback
}
