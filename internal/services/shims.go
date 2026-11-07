package services

import (
	"kiltro/internal/i18n"
	"os"
	"path/filepath"

	"kiltro/internal/config"
	"kiltro/internal/userpath"
)

// "Shims": bin\shims\php.bat y composer.bat. Son dos .bat chiquitos que llaman
// al php.exe de la versión elegida en Kiltro. La carpeta shims se agrega UNA
// vez al PATH del usuario; al cambiar de versión solo se reescriben los .bat,
// así que el PATH no se vuelve a tocar y las terminales nuevas ya usan la otra
// versión.
//
// El PHP de consola (php.exe) no recibe extension_dir como php-cgi (que lo
// recibe con -d al arrancar): sin ayuda busca las extensiones en C:\php\ext.
// Por eso los .bat definen PHP_INI_SCAN_DIR, que hace que PHP lea también
// bin\shims\conf.d\kiltro.ini (con el extension_dir correcto). Al ser una
// variable de entorno la heredan los PHP que lanza Composer (ej. los scripts
// "@php artisan ..."), que no recibirían un -d.

// ShimsStatus es lo que muestra la pantalla de configuración.
type ShimsStatus struct {
	Dir      string `json:"dir"`      // bin\shims
	InPath   bool   `json:"inPath"`   // ya está en el PATH del usuario
	PHP      string `json:"php"`      // carpeta de la versión a la que apuntan los .bat
	Composer bool   `json:"composer"` // hay bin\composer\composer.phar
	Error    string `json:"error"`
}

func shimsDir() string     { return config.Path("bin", "shims") }
func composerPhar() string { return config.Path("bin", "composer", "composer.phar") }

// writeShims (re)escribe los .bat para la versión de PHP elegida. Se llama al
// iniciar Kiltro (por si se movió la carpeta o se borró una versión) y al
// cambiar de versión. Devuelve la carpeta de PHP usada.
func writeShims() (string, error) {
	v, _, err := selectedPHP(listPHPVersions())
	if err != nil {
		return "", err
	}
	phpDir := config.Path("bin", "php", v.Name)
	phpExe := filepath.Join(phpDir, "php.exe")
	if _, err := os.Stat(phpExe); err != nil {
		return "", i18n.Err("shims.noPhpExe", "name", v.Name)
	}
	// El mismo php.ini que usa php-cgi (lo crea si falta, con las extensiones de Laravel).
	if _, err := ensurePhpIni(phpDir); err != nil {
		return "", err
	}

	dir := shimsDir()
	confDir := filepath.Join(dir, "conf.d")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		return "", i18n.Wrap(err, "err.create", "path", confDir)
	}
	ini := "; Generado por Kiltro (se reescribe solo).\n" +
		"extension_dir=\"" + filepath.Join(phpDir, "ext") + "\"\n"
	if err := writeIfChanged(filepath.Join(confDir, "kiltro.ini"), ini); err != nil {
		return "", err
	}

	// setlocal: el PHP_INI_SCAN_DIR queda solo para este comando, no ensucia
	// la terminal del usuario. %* = todos los argumentos tal cual.
	header := "@echo off\r\n" +
		"rem Generado por Kiltro: PHP elegido = " + v.Name + "\r\n" +
		"rem Se reescribe al cambiar la version de PHP en Kiltro (no editar).\r\n" +
		"setlocal\r\n" +
		"set \"PHP_INI_SCAN_DIR=" + confDir + "\"\r\n"
	if err := writeIfChanged(filepath.Join(dir, "php.bat"), header+"\""+phpExe+"\" %*\r\n"); err != nil {
		return "", err
	}

	composerBat := filepath.Join(dir, "composer.bat")
	if _, err := os.Stat(composerPhar()); err == nil {
		body := header + "\"" + phpExe + "\" \"" + composerPhar() + "\" %*\r\n"
		if err := writeIfChanged(composerBat, body); err != nil {
			return "", err
		}
	} else {
		os.Remove(composerBat) // sin composer.phar, que no quede un .bat que falla
	}
	return v.Name, nil
}

// writeIfChanged escribe el archivo solo si cambió (no tiene sentido reescribirlo en
// cada arranque, y así no choca con un comando que justo lo esté usando).
func writeIfChanged(path, content string) error {
	if old, err := os.ReadFile(path); err == nil && string(old) == content {
		return nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return i18n.Wrap(err, "err.write", "path", path)
	}
	return nil
}

// refreshShims reescribe los .bat y deja el resultado en el log.
func (m *ServiceManager) refreshShims() {
	name, err := writeShims()
	if err != nil {
		m.env.Log.Printf("[shims] no se pudieron generar php.bat/composer.bat: %v", err)
		return
	}
	m.env.Log.Printf("[shims] php.bat y composer.bat apuntan a %s", name)
}

// ShimsStatus dice si php/composer están disponibles en la terminal.
func (m *ServiceManager) ShimsStatus() ShimsStatus {
	st := ShimsStatus{Dir: shimsDir()}
	if v, _, err := selectedPHP(listPHPVersions()); err == nil {
		st.PHP = v.Name
	}
	_, err := os.Stat(composerPhar())
	st.Composer = err == nil
	inPath, err := userpath.Contains(st.Dir)
	st.InPath = inPath
	if err != nil {
		st.Error = "no se pudo leer el PATH del usuario: " + err.Error()
	}
	return st
}

// AddShimsToPath genera los .bat y agrega bin\shims al PATH del usuario.
func (m *ServiceManager) AddShimsToPath() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	name, err := writeShims()
	if err != nil {
		return err
	}
	added, err := userpath.Add(shimsDir())
	if err != nil {
		return i18n.Wrap(err, "shims.pathFailed")
	}
	if added {
		m.env.Log.Printf("[shims] %s agregado al PATH del usuario (php = %s)", shimsDir(), name)
	}
	return nil
}

// RemoveShimsFromPath quita bin\shims del PATH del usuario (los .bat quedan,
// pero ya no se encuentran escribiendo solo "php").
func (m *ServiceManager) RemoveShimsFromPath() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	removed, err := userpath.Remove(shimsDir())
	if err != nil {
		return i18n.Wrap(err, "shims.pathFailed")
	}
	if removed {
		m.env.Log.Printf("[shims] %s quitado del PATH del usuario", shimsDir())
	}
	return nil
}
