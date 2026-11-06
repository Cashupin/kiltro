// Package vhosts escanea la carpeta www/ y genera un vhost de Nginx por proyecto.
//
// Regla: cada subcarpeta directa de www/ es un proyecto y se sirve en <nombre>.test.
package vhosts

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// TLD es el dominio local. Siempre .test (nunca .dev ni .local, ver CLAUDE.md).
const TLD = ".test"

// Site es un proyecto encontrado en www/.
type Site struct {
	Name    string `json:"name"`    // nombre normalizado, ej. "mi-blog"
	Domain  string `json:"domain"`  // ej. "mi-blog.test"
	Folder  string `json:"folder"`  // ruta absoluta de la carpeta del proyecto
	Root    string `json:"root"`    // carpeta que sirve Nginx (Folder o Folder\public)
	Laravel bool   `json:"laravel"` // true si se detectó Laravel

	// Sitios "herramienta" que agrega Kiltro (ej. Adminer), no carpetas de www/.
	Tool bool `json:"tool"`
	// Si es true, Nginx solo atiende peticiones desde esta PC (127.0.0.1).
	LocalOnly bool `json:"localOnly"`
}

// Reserved son nombres que usa Kiltro para sus herramientas: una carpeta de
// www/ con ese nombre se ignora (si no, chocaría con la herramienta).
var Reserved = map[string]string{
	"adminer": "Adminer (gestor de base de datos)",
}

// Skipped es una carpeta que se ignoró, con el motivo (para loguear/mostrar).
type Skipped struct {
	Folder string `json:"folder"`
	Reason string `json:"reason"`
}

// Un nombre válido para un dominio: letras minúsculas, números y guiones,
// sin empezar ni terminar en guion, máximo 63 caracteres (límite de DNS).
var validName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Scan recorre las subcarpetas directas de wwwDir y devuelve los sitios
// (ordenados por nombre) y las carpetas ignoradas.
func Scan(wwwDir string) ([]Site, []Skipped, error) {
	entries, err := os.ReadDir(wwwDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil // sin carpeta www = sin sitios
		}
		return nil, nil, fmt.Errorf("no se pudo leer %s: %w", wwwDir, err)
	}

	var sites []Site
	var skipped []Skipped
	seen := map[string]string{} // nombre normalizado -> carpeta original (para detectar choques)

	for _, e := range entries {
		if !e.IsDir() {
			continue // archivos sueltos en www/ (ej. .gitkeep) no son proyectos
		}
		folder := e.Name()
		if strings.HasPrefix(folder, ".") {
			continue // carpetas ocultas (.git, .idea...) se ignoran sin avisar
		}

		name := normalizeName(folder)
		if !validName.MatchString(name) {
			skipped = append(skipped, Skipped{folder, "el nombre no sirve como dominio (usa letras, números y guiones)"})
			continue
		}
		if tool, reserved := Reserved[name]; reserved {
			skipped = append(skipped, Skipped{folder, fmt.Sprintf("el nombre %s%s está reservado para %s", name, TLD, tool)})
			continue
		}
		if other, dup := seen[name]; dup {
			skipped = append(skipped, Skipped{folder, fmt.Sprintf("choca con la carpeta %q (ambas serían %s%s)", other, name, TLD)})
			continue
		}
		seen[name] = folder

		path := filepath.Join(wwwDir, folder)
		site := Site{
			Name:   name,
			Domain: name + TLD,
			Folder: path,
			Root:   path,
		}
		if isLaravel(path) {
			site.Laravel = true
			site.Root = filepath.Join(path, "public")
		}
		sites = append(sites, site)
	}

	sort.Slice(sites, func(i, j int) bool { return sites[i].Name < sites[j].Name })
	return sites, skipped, nil
}

// normalizeName convierte el nombre de carpeta en un nombre de dominio:
// "Mi_Proyecto Nuevo" -> "mi-proyecto-nuevo".
func normalizeName(folder string) string {
	n := strings.ToLower(folder)
	n = strings.NewReplacer(" ", "-", "_", "-").Replace(n)
	return n
}

// isLaravel detecta un proyecto Laravel: tiene el archivo `artisan` en la raíz
// y un `public/index.php` (que es lo que hay que servir).
func isLaravel(dir string) bool {
	return fileExists(filepath.Join(dir, "artisan")) &&
		fileExists(filepath.Join(dir, "public", "index.php"))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
