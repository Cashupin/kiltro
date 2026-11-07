// Package certs genera los certificados HTTPS locales con mkcert
// (bin/mkcert/mkcert.exe).
//
// Cómo funciona mkcert:
//  1. Crea una "autoridad certificadora" (CA) propia: rootCA.pem + rootCA-key.pem.
//  2. `mkcert -install` la agrega a los certificados confiables de Windows (del
//     usuario, no requiere admin; Windows pide confirmación una vez).
//  3. Con esa CA firma un certificado por dominio. Como el navegador confía en
//     la CA, confía en los certificados: candado verde en https://proyecto.test.
//
// La CA vive en etc/ssl/ca (variable CAROOT), dentro de la carpeta de Kiltro.
// Su clave privada (rootCA-key.pem) permite firmar certificados para CUALQUIER
// sitio en esta PC: nunca compartirla. etc/ no va al repo.
package certs

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"kiltro/internal/config"
	"kiltro/internal/procutil"
)

// CARoot es la carpeta de la CA de Kiltro.
func CARoot() string { return config.Path("etc", "ssl", "ca") }

// certDir es donde quedan los certificados de cada dominio.
func certDir() string { return config.Path("etc", "ssl", "certs") }

// Files devuelve las rutas del certificado y la clave de un dominio
// (ej. etc/ssl/certs/demo.test.pem y demo.test-key.pem).
func Files(domain string) (cert, key string) {
	return filepath.Join(certDir(), domain+".pem"), filepath.Join(certDir(), domain+"-key.pem")
}

// Exe busca bin/mkcert/mkcert*.exe. Error si no está instalado.
func Exe() (string, error) {
	matches, _ := filepath.Glob(config.Path("bin", "mkcert", "mkcert*.exe"))
	if len(matches) == 0 {
		return "", fmt.Errorf("no se encontró mkcert: se esperaba %s", config.Path("bin", "mkcert", "mkcert.exe"))
	}
	return matches[len(matches)-1], nil
}

// Install crea la CA (si no existe) y la instala en los certificados de
// confianza de Windows. Windows muestra un aviso de seguridad: espera hasta
// que el usuario responda (máximo 5 minutos).
func Install(job *procutil.Job) error {
	if err := os.MkdirAll(CARoot(), 0o755); err != nil {
		return err
	}
	exe, err := Exe()
	if err != nil {
		return err
	}
	out, err := run(job, exe, true, 5*time.Minute, "-install")
	if err != nil {
		return fmt.Errorf("mkcert -install falló (¿rechazaste el aviso de Windows?): %s", out)
	}
	return nil
}

// Ensure crea los certificados que falten para los dominios dados. También
// rehace los que son más viejos que la CA (si se regeneró la CA, los
// certificados anteriores ya no sirven). Devuelve los dominios generados.
func Ensure(job *procutil.Job, domains []string) ([]string, error) {
	exe, err := Exe()
	if err != nil {
		return nil, err
	}
	caInfo, err := os.Stat(filepath.Join(CARoot(), "rootCA.pem"))
	if err != nil {
		return nil, errors.New("no existe la CA de Kiltro: activa HTTPS de nuevo")
	}
	if err := os.MkdirAll(certDir(), 0o755); err != nil {
		return nil, err
	}

	var created []string
	for _, d := range domains {
		cert, key := Files(d)
		if info, err := os.Stat(cert); err == nil && info.ModTime().After(caInfo.ModTime()) {
			if _, err := os.Stat(key); err == nil {
				continue // ya está y es de esta CA
			}
		}
		out, err := run(job, exe, false, 30*time.Second, "-cert-file", cert, "-key-file", key, d)
		if err != nil {
			return created, fmt.Errorf("no se pudo crear el certificado de %s: %s", d, out)
		}
		created = append(created, d)
	}
	return created, nil
}

// run ejecuta mkcert con CAROOT apuntando a la CA de Kiltro y espera a que termine.
// dialogs=true solo para -install (ver procutil.StartAllowDialogs).
func run(job *procutil.Job, exe string, dialogs bool, timeout time.Duration, args ...string) (string, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = config.Path()
	cmd.Env = append(os.Environ(),
		"CAROOT="+CARoot(),
		// Solo el almacén de Windows (Chrome, Edge y Firefox lo usan). Sin esto
		// mkcert también intentaría con Java o con el de Firefox (NSS).
		"TRUST_STORES=system",
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	start := procutil.Start
	if dialogs {
		start = procutil.StartAllowDialogs
	}
	if err := start(job, cmd); err != nil {
		return err.Error(), err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return strings.TrimSpace(out.String()), err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return "timeout", fmt.Errorf("mkcert no terminó tras %s", timeout)
	}
}
