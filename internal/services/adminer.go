package services

import (
	"fmt"
	"kiltro/internal/i18n"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kiltro/internal/config"
	"kiltro/internal/vhosts"
	"kiltro/templates"
)

// Adminer es el gestor de base de datos web: un solo archivo PHP
// (bin/adminer/adminer-<versión>-mysql.php) que Kiltro sirve en
// http://adminer.test usando el mismo Nginx + PHP de los proyectos.
// No es un "servicio" (no tiene proceso propio): es un sitio más.

// AdminerURL abre Adminer con el servidor y el usuario ya completados
// (por https si HTTPS está activado).
func AdminerURL() string {
	scheme := "http"
	if sslEnabled() {
		scheme = "https"
	}
	return scheme + "://adminer.test/?server=127.0.0.1&username=root"
}

// adminerFile busca el .php de Adminer; "" si no está instalado.
func adminerFile() string {
	matches, _ := filepath.Glob(config.Path("bin", "adminer", "adminer-*.php"))
	if len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	return matches[len(matches)-1]
}

// prepareAdminer escribe etc/adminer/index.php y devuelve el sitio de Adminer.
// Si Adminer no está en bin/adminer/, devuelve nil (no es un error: es opcional).
func prepareAdminer() (*vhosts.Site, error) {
	file := adminerFile()
	if file == "" {
		return nil, nil
	}

	dir := config.Path("etc", "adminer")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	// Envoltorio: Adminer permite personalizarlo definiendo adminer_object()
	// antes de incluirlo. Lo usamos para una sola cosa: permitir entrar como
	// root SIN contraseña (Adminer lo bloquea por defecto). Es seguro porque:
	//   - MySQL solo escucha en 127.0.0.1, y
	//   - el vhost de adminer.test solo atiende a 127.0.0.1 (LocalOnly).
	wrapper := fmt.Sprintf(`<?php
// Generado por Kiltro. NO EDITAR: se sobrescribe al recargar sitios.

function adminer_object() {
    class KiltroAdminer extends \Adminer\Adminer {
        // Permitir usuarios sin contraseña (root local de desarrollo).
        // Las credenciales las valida igual MySQL al conectar.
        function login($login, $password) {
            return true;
        }
    }
    return new KiltroAdminer;
}

include '%s';
`, phpSingleQuoted(filepath.ToSlash(file)))

	index := filepath.Join(dir, "index.php")
	if err := os.WriteFile(index, []byte(wrapper), 0o644); err != nil {
		return nil, i18n.Wrap(err, "err.write", "path", index)
	}

	if err := writeAdminerTheme(dir); err != nil {
		return nil, err
	}

	return &vhosts.Site{
		Name:      "adminer",
		Domain:    "adminer" + vhosts.TLD,
		Folder:    filepath.Dir(file),
		Root:      dir,
		Tool:      true,
		LocalOnly: true,
	}, nil
}

// writeAdminerTheme deja el tema de Kiltro junto a index.php. Adminer los
// detecta solo si están en esa carpeta, y los carga así:
//   - modo claro:  su CSS + adminer.css
//   - modo oscuro: su CSS + su dark.css + adminer-dark.css (¡sin adminer.css!)
//
// Por eso adminer-dark.css se arma como adminer.css (la forma: bordes, botones,
// tarjetas) + los colores oscuros de templates/adminer/adminer-dark.css.
func writeAdminerTheme(dir string) error {
	light, err := templates.FS.ReadFile("adminer/adminer.css")
	if err != nil {
		return err
	}
	darkColors, err := templates.FS.ReadFile("adminer/adminer-dark.css")
	if err != nil {
		return err
	}
	dark := string(light) + "\n" + string(darkColors)

	if err := os.WriteFile(filepath.Join(dir, "adminer.css"), light, 0o644); err != nil {
		return i18n.Wrap(err, "adminer.theme")
	}
	if err := os.WriteFile(filepath.Join(dir, "adminer-dark.css"), []byte(dark), 0o644); err != nil {
		return i18n.Wrap(err, "adminer.theme")
	}
	return nil
}

// phpSingleQuoted escapa un texto para meterlo entre comillas simples en PHP
// (ahí solo hay que escapar \ y '; a diferencia de las dobles, no interpreta $).
func phpSingleQuoted(s string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s)
}

// allSites son los sitios que Kiltro sirve: los proyectos de www/ más las
// herramientas (Adminer, si está instalado). Lo usan Nginx (vhosts), la UI y
// el archivo hosts, para que los tres vean siempre la misma lista.
func allSites() ([]vhosts.Site, []vhosts.Skipped, error) {
	sites, skipped, err := vhosts.Scan(config.Current().WWWDir())
	if err != nil {
		return nil, nil, err
	}
	adminer, err := prepareAdminer()
	if err != nil {
		return nil, nil, err
	}
	if adminer != nil {
		sites = append(sites, *adminer)
	}
	return sites, skipped, nil
}
