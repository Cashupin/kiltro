package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"kiltro/internal/certs"
	"kiltro/internal/config"
	"kiltro/internal/hosts"
	"kiltro/internal/procutil"
	"kiltro/internal/userpath"
)

// Modos "sin ventana" de kiltro.exe. Los usan el instalador/desinstalador
// (que corren como admin) y la propia app cuando se relanza con UAC. En estos
// modos NO se abre la ventana ni se arranca Wails: se hace la tarea y se sale.
//
//	kiltro.exe --write-hosts <archivo>   (app, con UAC) escribe el bloque del hosts
//	kiltro.exe --remove-ca <rootCA.pem>  (app, con UAC) quita la CA del almacén de la máquina
//	kiltro.exe --install-ca              (instalador, admin) crea la CA y la pone en la máquina
//	kiltro.exe --uninstall               (desinstalador, admin) quita CA, bloque del hosts y PATH
//
// --install-ca y --uninstall usan la carpeta del exe como raíz (en la versión
// instalada es la correcta). Los modos que lanza la app reciben las rutas como
// argumento, porque un proceso elevado no hereda KILTRO_ROOT.
//
// Como kiltro.exe es una app de ventanas (no de consola), no se ve nada de lo
// que se imprima: el resultado va a logs/kiltro.log y al código de salida
// (0 = todo bien), que es lo que lee el instalador.

// runCLI atiende esos modos. handled=false si no es ninguno (arrancar la app).
func runCLI(args []string) (handled bool, code int) {
	if len(args) < 1 {
		return false, 0
	}
	switch args[0] {
	case "--write-hosts":
		if len(args) < 2 {
			return true, 2
		}
		return true, hosts.RunElevated(args[1])

	case "--remove-ca":
		if len(args) < 2 {
			return true, 2
		}
		// <raíz>\etc\ssl\ca\rootCA.pem → <raíz>
		root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(args[1]))))
		l := cliLogger(root)
		if err := certs.RemoveFromMachine(args[1]); err != nil {
			l.Printf("[ssl] --remove-ca: %v", err)
			return true, 1
		}
		l.Printf("[ssl] --remove-ca: CA quitada del almacén de la máquina")
		return true, 0

	case "--install-ca":
		return true, installCA()

	case "--uninstall":
		return true, uninstall()
	}
	return false, 0
}

// installCA crea la CA de Kiltro (si no existe) y la agrega al almacén de la
// máquina, sin el aviso de seguridad de `mkcert -install`. Requiere admin.
func installCA() int {
	l := cliLogger(config.Path())
	job, err := procutil.NewJob() // mkcert corre dentro de un Job, como todo proceso hijo
	if err != nil {
		l.Printf("[instalador] --install-ca: %v", err)
		return 1
	}
	defer job.Close()

	if err := certs.CreateCA(job); err != nil {
		l.Printf("[instalador] --install-ca: %v", err)
		return 1
	}
	if err := certs.AddToMachine(); err != nil {
		l.Printf("[instalador] --install-ca: %v", err)
		return 1
	}
	l.Printf("[instalador] CA de Kiltro instalada en el almacén de la máquina (%s)", certs.CAFile())
	return 0
}

// uninstall deshace lo que Kiltro dejó FUERA de su carpeta: la CA en el
// almacén de la máquina, el bloque del hosts y bin\shims en el PATH. Sigue
// aunque falle un paso (para limpiar lo más posible) y devuelve 1 si alguno
// falló. Los archivos de la carpeta los borra el desinstalador.
func uninstall() int {
	l := cliLogger(config.Path())
	var errs []error

	if err := certs.RemoveFromMachine(certs.CAFile()); err != nil {
		errs = append(errs, err)
	} else {
		l.Printf("[desinstalador] CA quitada del almacén de la máquina")
	}

	if err := hosts.RemoveBlock(config.Path("etc", "hosts-backup")); err != nil {
		errs = append(errs, err)
	} else {
		l.Printf("[desinstalador] bloque de Kiltro quitado del hosts")
	}

	// PATH del usuario que corre el desinstalador (HKCU). Si el admin que
	// aceptó el UAC es otra cuenta, el PATH del usuario normal no se toca.
	if removed, err := userpath.Remove(config.Path("bin", "shims")); err != nil {
		errs = append(errs, err)
	} else if removed {
		l.Printf("[desinstalador] bin\\shims quitado del PATH del usuario")
	}

	if err := errors.Join(errs...); err != nil {
		l.Printf("[desinstalador] errores: %v", err)
		return 1
	}
	return 0
}

// cliLogger escribe en <raíz>/logs/kiltro.log (con fecha y hora, como el resto
// de Kiltro). Si no se puede abrir, descarta los mensajes.
func cliLogger(root string) *log.Logger {
	var w io.Writer = io.Discard
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o755); err == nil {
		if f, err := os.OpenFile(filepath.Join(root, "logs", "kiltro.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			w = f // se cierra solo al terminar el proceso
		}
	}
	return log.New(w, fmt.Sprintf("[pid %d] ", os.Getpid()), log.LstdFlags)
}
