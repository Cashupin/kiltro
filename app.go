package main

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"kiltro/internal/config"
	"kiltro/internal/services"
	"kiltro/internal/tray"
	"kiltro/internal/vhosts"
)

// App contiene los métodos que se exponen al frontend (bindings de Wails).
// Cada método público de App queda disponible en JS en frontend/wailsjs/go/main/App.
//
// Si un método devuelve un error, en JS la promesa se rechaza con el mensaje.
type App struct {
	ctx      context.Context
	manager  *services.ServiceManager
	initErr  error  // si el manager no pudo crearse, se guarda aquí para mostrarlo
	trayIcon []byte // .ico de la bandeja
}

// NewApp crea la estructura App.
func NewApp(trayIcon []byte) *App {
	return &App{trayIcon: trayIcon}
}

// startup lo llama Wails al arrancar.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Cada vez que un servicio cambia de estado, avisamos al frontend con un
	// evento (el frontend vuelve a pedir la lista con GetServices) y
	// actualizamos el menú de la bandeja.
	m, err := services.NewManager(func() {
		runtime.EventsEmit(a.ctx, "services:changed")
		tray.Update(a.trayServices())
	})
	if err != nil {
		a.initErr = err
		return
	}
	a.manager = m

	tray.Start(a.trayIcon, a.trayServices(), tray.Callbacks{
		Open:     a.showWindow,
		Toggle:   a.trayToggle,
		StartAll: a.trayStartAll,
		StopAll:  a.manager.StopAll,
		// runtime.Quit cierra Wails, que llama a shutdown (apaga los servicios).
		Quit: func() { runtime.Quit(a.ctx) },
	})
}

// shutdown lo llama Wails al cerrar la app ("Salir" en la bandeja): apaga
// todos los servicios limpio y quita el ícono de la bandeja.
func (a *App) shutdown(ctx context.Context) {
	if a.manager != nil {
		a.manager.Close()
	}
	tray.Stop()
}

// --- bandeja del sistema ---

// showWindow trae la ventana al frente (estaba escondida en la bandeja o minimizada).
func (a *App) showWindow() {
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
}

// onSecondInstance: alguien abrió kiltro.exe de nuevo; mostramos este.
func (a *App) onSecondInstance(_ options.SecondInstanceData) {
	a.showWindow()
}

// trayServices arma el estado de los servicios para el menú de la bandeja.
func (a *App) trayServices() []tray.Service {
	if a.manager == nil {
		return nil
	}
	labels := map[string]string{"php": "PHP", "mysql": "MySQL", "nginx": "Nginx"}
	var out []tray.Service
	for _, info := range a.manager.List() {
		label := labels[info.Name]
		if label == "" {
			label = info.Name
		}
		out = append(out, tray.Service{Name: info.Name, Label: label, Status: string(info.Status)})
	}
	return out
}

// trayToggle inicia o detiene un servicio desde el menú de la bandeja.
func (a *App) trayToggle(name string) {
	for _, info := range a.manager.List() {
		if info.Name != name {
			continue
		}
		var err error
		if info.Status == services.StatusRunning || info.Status == services.StatusStarting {
			err = a.StopService(name)
		} else {
			err = a.StartService(name)
		}
		if err != nil {
			a.reportError(err)
		}
		return
	}
}

func (a *App) trayStartAll() {
	err := a.manager.StartAll()
	go a.syncHostsInBackground() // Nginx pudo haber arrancado
	if err != nil {
		a.reportError(err)
	}
}

// reportError muestra un error que ocurrió desde la bandeja: abre la ventana
// (si no, el usuario no se entera) y le pasa el mensaje a la UI.
func (a *App) reportError(err error) {
	a.showWindow()
	runtime.EventsEmit(a.ctx, "app:error", err.Error())
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

// GetServices devuelve el estado de todos los servicios.
func (a *App) GetServices() ([]services.Info, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.manager.List(), nil
}

// StartService arranca un servicio por nombre (ej. "nginx").
func (a *App) StartService(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if err := a.manager.Start(name); err != nil {
		return err
	}
	if name == "nginx" {
		go a.syncHostsInBackground()
	}
	return nil
}

// StopService detiene un servicio por nombre.
func (a *App) StopService(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.manager.Stop(name)
}

// GetPHPVersions devuelve las versiones de PHP instaladas (la elegida viene marcada).
func (a *App) GetPHPVersions() ([]services.PHPVersion, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.manager.PHPVersions(), nil
}

// SetPHPVersion cambia la versión de PHP (reinicia PHP si está corriendo).
func (a *App) SetPHPVersion(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.manager.SetPHPVersion(name)
}

// SitesResult es lo que recibe la UI: los sitios y las carpetas ignoradas.
type SitesResult struct {
	Sites   []vhosts.Site    `json:"sites"`
	Skipped []vhosts.Skipped `json:"skipped"`
}

// GetSites devuelve los proyectos encontrados en www/.
func (a *App) GetSites() (SitesResult, error) {
	if err := a.ready(); err != nil {
		return SitesResult{}, err
	}
	sites, skipped, err := a.manager.Sites()
	return SitesResult{Sites: sites, Skipped: skipped}, err
}

// ReloadSites regenera los vhosts y recarga nginx si está corriendo.
func (a *App) ReloadSites() error {
	if err := a.ready(); err != nil {
		return err
	}
	if _, err := a.manager.ReloadSites(); err != nil {
		return err
	}
	go a.syncHostsInBackground()
	return nil
}

// OpenAdminer abre el gestor de base de datos en el navegador, con el
// servidor y el usuario root ya completados.
func (a *App) OpenAdminer() {
	runtime.BrowserOpenURL(a.ctx, services.AdminerURL)
}

// GetHostsStatus dice si el archivo hosts tiene los dominios de www/.
func (a *App) GetHostsStatus() (services.HostsStatus, error) {
	if err := a.ready(); err != nil {
		return services.HostsStatus{}, err
	}
	return a.manager.HostsStatus(), nil
}

// SyncHosts actualiza el hosts (botón "Actualizar hosts"). Puede mostrar el UAC.
func (a *App) SyncHosts() error {
	if err := a.ready(); err != nil {
		return err
	}
	err := a.manager.SyncHosts()
	runtime.EventsEmit(a.ctx, "hosts:changed")
	return err
}

// syncHostsInBackground actualiza el hosts sin bloquear el botón que lo
// disparó (el usuario puede tardar en responder el aviso de UAC). Al terminar
// avisa al frontend; el error queda en HostsStatus.Error.
func (a *App) syncHostsInBackground() {
	_ = a.manager.SyncHosts()
	runtime.EventsEmit(a.ctx, "hosts:changed")
}

func (a *App) ready() error {
	if a.initErr != nil {
		return a.initErr
	}
	if a.manager == nil {
		return errors.New("la app todavía no terminó de iniciar")
	}
	return nil
}
