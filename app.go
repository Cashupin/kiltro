package main

import (
	"context"

	"kiltro/internal/config"
)

// App contiene los métodos que se exponen al frontend (bindings de Wails).
// Cada método público de App queda disponible en JS en frontend/wailsjs/go/main/App.
type App struct {
	ctx context.Context
}

// NewApp crea la estructura App.
func NewApp() *App {
	return &App{}
}

// startup lo llama Wails al arrancar. Guardamos el contexto para poder usar
// los métodos de runtime de Wails más adelante.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// GetAppRoot devuelve la carpeta raíz de la app (ver config.AppRoot).
// Si hubo un error, se devuelve el mensaje para que se vea en la UI.
func (a *App) GetAppRoot() string {
	root, err := config.AppRoot()
	if err != nil {
		return "error: " + err.Error()
	}
	return root
}
