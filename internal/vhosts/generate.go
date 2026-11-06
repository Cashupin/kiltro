package vhosts

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"kiltro/templates"
)

// Options son los datos que la plantilla necesita además de cada Site.
type Options struct {
	SitesDir    string // carpeta donde se escriben los .conf (etc/nginx/sites)
	FastcgiConf string // ruta del fastcgi.conf que trae nginx
	Port        int    // puerto HTTP (80)
}

// Datos que recibe vhost.conf.tmpl. Las rutas van con "/" (Nginx las acepta
// así en Windows y evita problemas con "\" dentro de comillas).
type templateData struct {
	Domain      string
	Folder      string
	Root        string
	Laravel     bool
	Port        int
	FastcgiConf string
}

// Generate escribe un <dominio>.conf por sitio en opts.SitesDir y borra los
// .conf de sitios que ya no existen. Toda la carpeta es de Kiltro: no poner
// archivos a mano ahí.
func Generate(sites []Site, opts Options) error {
	raw, err := templates.FS.ReadFile("nginx/vhost.conf.tmpl")
	if err != nil {
		return err
	}
	tmpl, err := template.New("vhost").Parse(string(raw))
	if err != nil {
		return fmt.Errorf("plantilla de vhost inválida: %w", err)
	}

	if err := os.MkdirAll(opts.SitesDir, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear %s: %w", opts.SitesDir, err)
	}

	keep := map[string]bool{}
	for _, s := range sites {
		var buf bytes.Buffer
		err := tmpl.Execute(&buf, templateData{
			Domain:      s.Domain,
			Folder:      filepath.ToSlash(s.Folder),
			Root:        filepath.ToSlash(s.Root),
			Laravel:     s.Laravel,
			Port:        opts.Port,
			FastcgiConf: filepath.ToSlash(opts.FastcgiConf),
		})
		if err != nil {
			return fmt.Errorf("vhost de %s: %w", s.Domain, err)
		}

		file := s.Domain + ".conf"
		keep[file] = true
		if err := os.WriteFile(filepath.Join(opts.SitesDir, file), buf.Bytes(), 0o644); err != nil {
			return fmt.Errorf("no se pudo escribir el vhost de %s: %w", s.Domain, err)
		}
	}

	// Borrar vhosts de carpetas que ya no están en www/.
	entries, err := os.ReadDir(opts.SitesDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") || keep[e.Name()] {
			continue
		}
		if err := os.Remove(filepath.Join(opts.SitesDir, e.Name())); err != nil {
			return fmt.Errorf("no se pudo borrar el vhost viejo %s: %w", e.Name(), err)
		}
	}
	return nil
}
