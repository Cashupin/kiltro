package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"kiltro/internal/hosts"
)

//go:embed all:frontend/dist
var assets embed.FS

// Ícono de la bandeja (el mismo del .exe). Por ahora el de la plantilla de Wails.
//
//go:embed build/windows/icon.ico
var trayIcon []byte

func main() {
	// Modo "escribir hosts": la app se relanzó a sí misma como admin (UAC) solo
	// para modificar el archivo hosts. Se hace eso y se sale, SIN abrir ventana.
	// Ver internal/hosts/elevate.go.
	if len(os.Args) >= 3 && os.Args[1] == "--write-hosts" {
		os.Exit(hosts.RunElevated(os.Args[2]))
	}

	// Create an instance of the app structure
	app := NewApp(trayIcon)

	err := wails.Run(&options.App{
		Title:  "Kiltro",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,

		// La X esconde la ventana en la bandeja: los servicios siguen corriendo.
		// Para cerrar de verdad: "Salir" en el menú de la bandeja.
		HideWindowOnClose: true,

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
