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

func main() {
	// Modo "escribir hosts": la app se relanzó a sí misma como admin (UAC) solo
	// para modificar el archivo hosts. Se hace eso y se sale, SIN abrir ventana.
	// Ver internal/hosts/elevate.go.
	if len(os.Args) >= 3 && os.Args[1] == "--write-hosts" {
		os.Exit(hosts.RunElevated(os.Args[2]))
	}

	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
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
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
