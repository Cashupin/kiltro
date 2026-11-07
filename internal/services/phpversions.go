package services

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"kiltro/internal/config"
)

// Cada versión de PHP es una carpeta bin/php/php-<versión>-.../ con su propio
// php-cgi.exe, su php.ini y su ext/ (las DLL de extensiones son de ESA versión
// exacta, por eso nunca se comparten entre carpetas).

// PHPVersion describe una carpeta de PHP instalada. Se envía a la UI.
type PHPVersion struct {
	Name     string `json:"name"`     // nombre de la carpeta, ej. "php-8.4.27-1-nts-Win32-vs17-x64"
	Version  string `json:"version"`  // ej. "8.4.27" ("" si el nombre no trae versión)
	NTS      bool   `json:"nts"`      // Non Thread Safe: la recomendada para FastCGI
	Selected bool   `json:"selected"` // la que se usa al iniciar PHP
}

// "php-8.4.27-1-nts-..." → 8, 4, 27
var phpVersionRe = regexp.MustCompile(`^php-(\d+)\.(\d+)\.(\d+)`)

// phpVersionParts devuelve [mayor, menor, parche] para poder ordenar por
// número (con orden alfabético "8.10" quedaría antes que "8.9").
func phpVersionParts(name string) ([3]int, bool) {
	var parts [3]int
	m := phpVersionRe.FindStringSubmatch(name)
	if m == nil {
		return parts, false
	}
	for i := range parts {
		parts[i], _ = strconv.Atoi(m[i+1])
	}
	return parts, true
}

// listPHPVersions busca las carpetas de bin/php/ que tienen php-cgi.exe,
// ordenadas de la más vieja a la más nueva. Selected viene en false.
func listPHPVersions() []PHPVersion {
	matches, _ := filepath.Glob(config.Path("bin", "php", "php-*", "php-cgi.exe"))

	var list []PHPVersion
	for _, exe := range matches {
		dir := filepath.Dir(exe)
		v := PHPVersion{Name: filepath.Base(dir)}
		if p, ok := phpVersionParts(v.Name); ok {
			v.Version = fmt.Sprintf("%d.%d.%d", p[0], p[1], p[2])
		}
		// La build TS (thread safe) trae php8ts.dll (o php7ts.dll...); la NTS no.
		ts, _ := filepath.Glob(filepath.Join(dir, "php*ts.dll"))
		v.NTS = len(ts) == 0
		list = append(list, v)
	}

	sort.SliceStable(list, func(i, j int) bool {
		a, okA := phpVersionParts(list[i].Name)
		b, okB := phpVersionParts(list[j].Name)
		if okA != okB {
			return !okA // las que no tienen versión en el nombre van primero (= "más viejas")
		}
		if a != b {
			for k := range a {
				if a[k] != b[k] {
					return a[k] < b[k]
				}
			}
		}
		return list[i].Name < list[j].Name
	})
	return list
}

// PHPVersions devuelve las versiones instaladas, marcando la que está elegida.
func PHPVersions() []PHPVersion {
	list := listPHPVersions()
	if chosen, _, err := selectedPHP(list); err == nil {
		for i := range list {
			list[i].Selected = list[i].Name == chosen.Name
		}
	}
	return list
}

// selectedPHP decide qué versión usar:
//   - la guardada en config.json, si esa carpeta todavía existe;
//   - si no, la más nueva. En ese caso warning explica por qué (ej. se borró
//     la carpeta elegida), para dejarlo en el log.
func selectedPHP(list []PHPVersion) (v PHPVersion, warning string, err error) {
	if len(list) == 0 {
		return v, "", fmt.Errorf("no se encontró PHP: se esperaba %s",
			config.Path("bin", "php", "php-<versión>", "php-cgi.exe"))
	}
	latest := list[len(list)-1]

	cfg, err := config.Load()
	if err != nil {
		// config.json roto: no impide usar PHP, pero hay que avisar.
		return latest, err.Error() + "; se usa " + latest.Name, nil
	}
	if cfg.PHP == "" {
		return latest, "", nil
	}
	for _, x := range list {
		if x.Name == cfg.PHP {
			return x, "", nil
		}
	}
	return latest, fmt.Sprintf("la versión elegida (%s) ya no está en bin\\php; se usa %s", cfg.PHP, latest.Name), nil
}

// phpCgiExe devuelve el php-cgi.exe de la versión elegida (y un aviso, si
// hubo que usar otra).
func phpCgiExe() (exe, warning string, err error) {
	v, warning, err := selectedPHP(listPHPVersions())
	if err != nil {
		return "", "", err
	}
	exe = config.Path("bin", "php", v.Name, "php-cgi.exe")
	if _, err := os.Stat(exe); err != nil {
		return "", "", fmt.Errorf("no se encontró %s: %w", exe, err)
	}
	return exe, warning, nil
}

// savePHPVersion guarda la versión elegida en config.json (validando que exista).
func savePHPVersion(name string) error {
	found := false
	for _, v := range listPHPVersions() {
		if v.Name == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no existe la versión de PHP %q en bin\\php", name)
	}
	return config.Update(func(s *config.Settings) { s.PHP = name })
}
