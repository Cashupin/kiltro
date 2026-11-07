package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"kiltro/internal/config"
	"kiltro/internal/logview"
	"kiltro/internal/procutil"
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
	labels := map[string]string{"php": "PHP", "mysql": "MySQL", "redis": "Redis", "nginx": "Nginx"}
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
	if err := a.StartAll(); err != nil {
		a.reportError(err)
	}
}

// StartAll inicia todos los servicios en orden (botón "Iniciar todo" y bandeja).
// Si uno falla, sigue con los demás y devuelve los errores juntos.
func (a *App) StartAll() error {
	if err := a.ready(); err != nil {
		return err
	}
	err := a.manager.StartAll()
	go a.syncHostsInBackground() // Nginx pudo haber arrancado
	return err
}

// StopAll detiene todos los servicios (botón "Detener todo").
func (a *App) StopAll() error {
	if err := a.ready(); err != nil {
		return err
	}
	a.manager.StopAll()
	return nil
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
// servidor y el usuario root ya completados. Antes inicia lo que haga falta
// (PHP, MySQL y Nginx, en ese orden) y se asegura de que adminer.test esté en
// el hosts: si no, el navegador abriría una página que no carga.
func (a *App) OpenAdminer() error {
	if err := a.ready(); err != nil {
		return err
	}
	names := map[string]string{"php": "PHP", "mysql": "MySQL", "nginx": "Nginx"}
	for _, name := range []string{"php", "mysql", "nginx"} {
		if err := a.manager.Start(name); err != nil { // si ya corre, no hace nada
			return fmt.Errorf("no se pudo iniciar %s para abrir el gestor: %w", names[name], err)
		}
	}
	// Puede mostrar el aviso de UAC (solo si faltaba adminer.test en el hosts).
	err := a.manager.SyncHosts()
	runtime.EventsEmit(a.ctx, "hosts:changed")
	if err != nil {
		return err
	}
	runtime.BrowserOpenURL(a.ctx, services.AdminerURL())
	return nil
}

// --- Sitios ---

// OpenSiteFolder abre la carpeta de un proyecto en el Explorador.
func (a *App) OpenSiteFolder(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.manager.OpenSiteFolder(name)
}

// OpenSiteTerminal abre una terminal en la carpeta de un proyecto.
func (a *App) OpenSiteTerminal(name string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.manager.OpenSiteTerminal(name)
}

// CreateSite crea la carpeta de un proyecto nuevo en www/ y recarga los
// sitios. Devuelve el dominio (ej. "mi-tienda.test").
func (a *App) CreateSite(name string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	domain, err := a.manager.CreateSite(name)
	if err == nil {
		go a.syncHostsInBackground() // dominio nuevo: hay que agregarlo al hosts (UAC)
	}
	return domain, err
}

// SettingsView es lo que recibe la pantalla de configuración.
type SettingsView struct {
	Settings   config.Settings `json:"settings"`
	DefaultWWW string          `json:"defaultWww"` // carpeta www por defecto (si Settings.WWW está vacío)
	Error      string          `json:"error"`      // ej. config.json roto (se muestran los valores por defecto)
}

// GetSettings devuelve la configuración actual (con los valores por defecto completados).
func (a *App) GetSettings() SettingsView {
	s, err := config.Load()
	v := SettingsView{Settings: s, DefaultWWW: config.Path("www")}
	if err != nil {
		v.Error = err.Error()
	}
	return v
}

// SaveSettings guarda la configuración y reinicia lo que haga falta.
// Devuelve los servicios reiniciados (ej. ["nginx"]).
func (a *App) SaveSettings(s config.Settings) ([]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	oldWWW := config.Current().WWWDir()
	restarted, err := a.manager.SaveSettings(s)
	if config.Current().WWWDir() != oldWWW {
		// Otra carpeta de proyectos = otros dominios en el hosts.
		go a.syncHostsInBackground()
	}
	return restarted, err
}

// ChooseFolder abre el diálogo de Windows para elegir una carpeta. "" si se canceló.
func (a *App) ChooseFolder(current string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Carpeta de proyectos",
		DefaultDirectory: current,
	})
}

// UninstallCA quita la CA de HTTPS de Windows (desactiva HTTPS si estaba activo).
func (a *App) UninstallCA() error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.manager.UninstallCA()
}

// --- Logs ---

// GetLogs devuelve los logs que se pueden ver (para el desplegable).
func (a *App) GetLogs() []logview.Log {
	return logview.List()
}

// ReadLog devuelve las últimas 500 líneas del log indicado.
func (a *App) ReadLog(id string) (logview.Result, error) {
	return logview.Tail(id, 500)
}

// ClearLog vacía un log (la UI pide confirmación antes).
func (a *App) ClearLog(id string) error {
	if err := logview.Clear(id); err != nil {
		return err
	}
	if a.manager != nil {
		a.manager.LogPrintf("[logs] log %q vaciado desde la UI", id)
	}
	return nil
}

// OpenLogsFolder abre la carpeta logs\ en el Explorador de Windows.
//
// Con procutil.ShellOpen (y no como proceso hijo): así el Explorador no queda
// dentro del Job Object de Kiltro (si no, esa ventana se cerraría al salir de Kiltro).
func (a *App) OpenLogsFolder() error {
	dir := config.Path("logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return procutil.ShellOpen(dir, "", "")
}

// GetSSLStatus dice si HTTPS está activado y si mkcert está instalado.
func (a *App) GetSSLStatus() (services.SSLStatus, error) {
	if err := a.ready(); err != nil {
		return services.SSLStatus{}, err
	}
	return a.manager.SSLStatus(), nil
}

// EnableSSL activa HTTPS (Windows muestra un aviso para confiar en la CA).
func (a *App) EnableSSL() error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.manager.EnableSSL()
}

// DisableSSL vuelve a solo HTTP.
func (a *App) DisableSSL() error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.manager.DisableSSL()
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
