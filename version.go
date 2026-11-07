package main

// version es la versión de Kiltro. La pone tools/release.ps1 al compilar:
//
//	wails build -ldflags "-X main.version=0.1.0"
//
// (el valor sale de info.productVersion en wails.json). En `wails dev` y en un
// `wails build` sin ese flag queda "dev".
var version = "dev"

// GetVersion devuelve la versión de Kiltro (sección "Acerca de").
func (a *App) GetVersion() string {
	return version
}
