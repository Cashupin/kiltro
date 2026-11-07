// Package hosts lee y modifica el archivo hosts de Windows.
//
// Reglas (ver CLAUDE.md):
//   - Solo se toca lo que está entre "# KILTRO START" y "# KILTRO END".
//     Nada fuera del bloque se modifica jamás.
//   - Escribir el hosts requiere admin. La app NO corre como admin: para escribir
//     se relanza el propio kiltro.exe elevado (ver elevate.go).
//   - Antes de escribir se hace un backup.
//
// Las funciones de este archivo son "puras" (texto entra, texto sale), para
// poder probarlas sin tocar el hosts real.
package hosts

import (
	"kiltro/internal/i18n"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	StartMarker = "# KILTRO START"
	EndMarker   = "# KILTRO END"
)

// Solo se aceptan dominios <nombre>.test. Esto es una medida de seguridad: el
// proceso elevado lee la lista desde un archivo; aunque alguien lo edite, no
// puede meter entradas como "banco.com" en el hosts.
var validDomain = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.test$`)

// ValidDomain dice si un dominio puede ir en el bloque de Kiltro.
func ValidDomain(d string) bool {
	return validDomain.MatchString(d)
}

// SystemPath es la ruta del archivo hosts de Windows.
func SystemPath() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", "drivers", "etc", "hosts")
}

// Read lee el archivo hosts (leerlo NO requiere admin).
func Read() (string, error) {
	data, err := os.ReadFile(SystemPath())
	if err != nil {
		return "", i18n.Wrap(err, "hosts.readFailed")
	}
	return string(data), nil
}

// BlockDomains devuelve los dominios que hay hoy dentro del bloque de Kiltro
// (ordenados). Si no hay bloque, devuelve nil.
func BlockDomains(content string) []string {
	lines := splitLines(content)
	start, end, err := findBlock(lines)
	if err != nil || start < 0 {
		return nil
	}

	var domains []string
	for _, line := range lines[start+1 : end] {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Formato: "127.0.0.1 dominio1 dominio2 ..."
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			domains = append(domains, fields[1:]...)
		}
	}
	sort.Strings(domains)
	return domains
}

// UpToDate dice si el bloque ya tiene exactamente esos dominios.
func UpToDate(content string, domains []string) bool {
	want := append([]string(nil), domains...)
	sort.Strings(want)
	have := BlockDomains(content)
	if len(want) != len(have) {
		return false
	}
	for i := range want {
		if want[i] != have[i] {
			return false
		}
	}
	return true
}

// ReplaceBlock devuelve el contenido del hosts con el bloque de Kiltro
// reemplazado por uno con esos dominios:
//   - Si el bloque existe, se reemplaza en su lugar.
//   - Si no existe, se agrega al final.
//   - Si domains está vacío, el bloque se quita.
//
// Las líneas fuera del bloque quedan idénticas, y se respeta el tipo de salto
// de línea del archivo (Windows usa CRLF).
func ReplaceBlock(content string, domains []string) (string, error) {
	for _, d := range domains {
		if !ValidDomain(d) {
			return "", i18n.Err("hosts.badDomain", "domain", d)
		}
	}

	// Salto de línea: CRLF (lo normal en Windows), salvo que el archivo ya use LF.
	nl := "\r\n"
	if strings.Contains(content, "\n") && !strings.Contains(content, "\r\n") {
		nl = "\n"
	}

	lines := splitLines(content)
	start, end, err := findBlock(lines)
	if err != nil {
		return "", err
	}
	block := buildBlock(domains)

	var out []string
	switch {
	case start >= 0:
		// Reemplazar las líneas start..end (inclusive) por el bloque nuevo.
		out = append(out, lines[:start]...)
		out = append(out, block...)
		out = append(out, lines[end+1:]...)
		return strings.Join(out, nl), nil

	case len(block) == 0:
		return content, nil // no hay bloque y no hace falta: no se toca nada

	default:
		// Agregar al final, sin modificar lo que ya hay.
		result := content
		if result != "" && !strings.HasSuffix(result, "\n") {
			result += nl
		}
		return result + strings.Join(block, nl) + nl, nil
	}
}

// buildBlock arma las líneas del bloque. Sin dominios no hay bloque.
//
// Solo se usa 127.0.0.1 (IPv4) y no ::1: Nginx escucha en IPv4, y si el hosts
// dijera ::1 el navegador probaría primero IPv6 y tardaría en caer a IPv4.
func buildBlock(domains []string) []string {
	if len(domains) == 0 {
		return nil
	}
	sorted := append([]string(nil), domains...)
	sort.Strings(sorted)

	lines := []string{
		StartMarker,
		"# Administrado por Kiltro: no editar a mano entre estas marcas.",
	}
	for _, d := range sorted {
		lines = append(lines, "127.0.0.1 "+d)
	}
	return append(lines, EndMarker)
}

// splitLines separa en líneas sin los "\r". Un archivo que termina en salto de
// línea deja un último elemento "", que al unir de nuevo reproduce ese salto.
func splitLines(content string) []string {
	return strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
}

// findBlock devuelve los índices de las líneas START y END (-1, -1 si no hay
// bloque). Si hay un START sin END, es un error: preferimos no adivinar y no
// tocar nada antes que borrar líneas que no son nuestras.
func findBlock(lines []string) (start, end int, err error) {
	start, end = -1, -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case StartMarker:
			if start >= 0 {
				return -1, -1, i18n.Err("hosts.twoStart", "marker", StartMarker)
			}
			start = i
		case EndMarker:
			if start < 0 {
				return -1, -1, i18n.Err("hosts.unpaired", "a", EndMarker, "b", StartMarker)
			}
			if end < 0 {
				end = i
			}
		}
	}
	if start >= 0 && end < 0 {
		return -1, -1, i18n.Err("hosts.unpaired", "a", StartMarker, "b", EndMarker)
	}
	return start, end, nil
}
