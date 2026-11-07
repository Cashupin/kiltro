package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// Ícono de la bandeja (el mismo del .exe).
//
//go:embed build/windows/icon.ico
var trayIcon []byte

func main() {
	// Modos sin ventana (--write-hosts, --install-ca, --uninstall...): se hace
	// la tarea y se sale, SIN arrancar Wails. Ver cli.go.
	if handled, code := runCLI(os.Args[1:]); handled {
		os.Exit(code)
	}

	// Create an instance of the app structure
	app := NewApp(trayIcon)

	// --tray: arrancar escondido en la bandeja (lo usa "Iniciar con Windows" del
	// instalador). La ventana se abre desde el ícono, como siempre.
	startHidden := len(os.Args) > 1 && os.Args[1] == "--tray"

	err := wails.Run(&options.App{
		Title:  "Kiltro",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 15, G: 22, B: 35, A: 1}, // = --bg de style.css
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,

		// La X esconde la ventana en la bandeja: los servicios siguen corriendo.
		// Para cerrar de verdad: "Salir" en el menú de la bandeja.
		HideWindowOnClose: true,
		StartHidden:       startHidden,

		// Un solo Kiltro a la vez (dos pelearían por los puertos y el Job).
		// Si se abre otra vez el .exe, se muestra la ventana del que ya corre.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "kiltro-3f6b2c1e-8d4a-4f0e-9a51-7c2d9e4b1a60",
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
