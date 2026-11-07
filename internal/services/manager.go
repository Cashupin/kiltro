package services

import (
	"errors"
	"fmt"
	"io"
	"kiltro/internal/i18n"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kiltro/internal/certs"
	"kiltro/internal/config"
	"kiltro/internal/hosts"
	"kiltro/internal/procutil"
	"kiltro/internal/vhosts"
)

// Env es lo que el manager le pasa a cada servicio al crearlo: lo que todos
// necesitan compartir.
type Env struct {
	Job    *procutil.Job // el único Job Object de la app; todo proceso hijo va aquí
	Log    *log.Logger   // escribe en logs/kiltro.log
	Notify func()        // el servicio la llama cuando cambia su estado (ej. se cayó solo)
}

// Info es el resumen de un servicio que se envía al frontend.
type Info struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Error  string `json:"error"`
	Ports  []int  `json:"ports"`
	Detail string `json:"detail"` // texto extra opcional (ver Detailer)
}

// ServiceManager coordina todos los servicios: chequea puertos antes de
// arrancar, loguea cada acción y los apaga todos al cerrar la app.
type ServiceManager struct {
	env      *Env
	services []Service // en orden de arranque
	logFile  *os.File
	nginx    *Nginx // acceso directo para lo que es propio de nginx (sitios)

	// opMu evita que dos Start/Stop corran a la vez (ej. doble click en el botón).
	// List() no lo usa, para que la UI pueda leer estados mientras se detiene algo.
	opMu sync.Mutex

	hostsMu  sync.Mutex // un solo aviso de UAC a la vez
	hostsErr string     // error del último intento de actualizar el hosts
}

// NewManager crea el manager, abre logs/kiltro.log, crea el Job Object y
// registra los servicios. onChange se llama cada vez que cambia algún estado.
func NewManager(onChange func()) (*ServiceManager, error) {
	if err := os.MkdirAll(config.Path("logs"), 0o755); err != nil {
		return nil, i18n.Wrap(err, "err.create", "path", config.Path("logs"))
	}
	f, err := os.OpenFile(config.Path("logs", "kiltro.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, i18n.Wrap(err, "err.open", "path", config.Path("logs", "kiltro.log"))
	}
	// Al archivo y a la consola (la consola solo se ve en `wails dev`).
	logger := log.New(io.MultiWriter(f, os.Stderr), "", log.LstdFlags)

	job, err := procutil.NewJob()
	if err != nil {
		f.Close()
		return nil, err
	}

	if onChange == nil {
		onChange = func() {}
	}
	env := &Env{Job: job, Log: logger, Notify: onChange}

	nginx := NewNginx(env)
	m := &ServiceManager{
		env:     env,
		logFile: f,
		nginx:   nginx,
		// Orden de arranque: PHP, MySQL y Redis antes que Nginx (Nginx es la "puerta"
		// de entrada; cuando atiende, lo demás ya está listo).
		// StopAll los detiene en orden inverso.
		services: []Service{
			NewPHP(env),
			NewMySQL(env),
			NewRedis(env),
			nginx,
		},
	}
	logger.Printf("Kiltro iniciado. Raíz: %s", config.Path())
	m.refreshShims()
	return m, nil
}

// LogPrintf escribe una línea en logs/kiltro.log (para acciones de la UI que
// no son de un servicio, ej. vaciar un log).
func (m *ServiceManager) LogPrintf(format string, args ...any) {
	m.env.Log.Printf(format, args...)
}

// get busca un servicio por nombre.
func (m *ServiceManager) get(name string) (Service, error) {
	for _, s := range m.services {
		if s.Name() == name {
			return s, nil
		}
	}
	return nil, i18n.Err("err.unknownService", "name", name)
}

// Start arranca un servicio, verificando antes que sus puertos estén libres.
func (m *ServiceManager) Start(name string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	s, err := m.get(name)
	if err != nil {
		return err
	}
	return m.start(s)
}

// start arranca un servicio (sin tomar opMu: lo toma quien llama).
func (m *ServiceManager) start(s Service) error {
	if st := s.Status(); st == StatusRunning || st == StatusStarting {
		return nil // ya está corriendo
	}

	for _, port := range s.Ports() {
		if err := procutil.CheckPortFree(port); err != nil {
			m.env.Log.Printf("[%s] no se inicia: %v", s.Name(), err)
			return err
		}
	}

	m.env.Log.Printf("[%s] iniciando...", s.Name())
	if err := s.Start(); err != nil {
		m.env.Log.Printf("[%s] error al iniciar: %v", s.Name(), err)
		return err
	}
	m.env.Log.Printf("[%s] iniciado", s.Name())
	return nil
}

// PHPVersions devuelve las versiones de PHP instaladas en bin/php/.
func (m *ServiceManager) PHPVersions() []PHPVersion {
	return PHPVersions()
}

// SetPHPVersion guarda la versión de PHP elegida (en config.json) y, si PHP
// está corriendo, lo reinicia con la nueva. Nginx no se toca: los workers
// nuevos escuchan en los mismos puertos 9000-9003.
func (m *ServiceManager) SetPHPVersion(name string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	if err := savePHPVersion(name); err != nil {
		return err
	}
	m.env.Log.Printf("[php] versión elegida: %s", name)
	m.refreshShims() // php.bat y composer.bat apuntan a la nueva versión

	php, err := m.get("php")
	if err != nil {
		return err
	}
	if st := php.Status(); st != StatusRunning && st != StatusStarting {
		m.env.Notify()
		return nil // detenido: se usará la próxima vez que se inicie
	}
	if err := m.stop(php); err != nil {
		return err
	}
	return m.start(php)
}

// Stop detiene un servicio de forma limpia.
func (m *ServiceManager) Stop(name string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	s, err := m.get(name)
	if err != nil {
		return err
	}
	return m.stop(s)
}

func (m *ServiceManager) stop(s Service) error {
	if st := s.Status(); st != StatusRunning && st != StatusStarting {
		return nil
	}
	m.env.Log.Printf("[%s] deteniendo...", s.Name())
	if err := s.Stop(); err != nil {
		m.env.Log.Printf("[%s] error al detener: %v", s.Name(), err)
		return err
	}
	m.env.Log.Printf("[%s] detenido", s.Name())
	return nil
}

// StartAll inicia todos los servicios en orden (PHP, MySQL, Redis, Nginx). Si uno
// falla, sigue con los demás y devuelve todos los errores juntos.
func (m *ServiceManager) StartAll() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	var errs []error
	for _, s := range m.services {
		if err := m.start(s); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.Name(), err))
		}
	}
	return errors.Join(errs...)
}

// StopAll detiene todos los servicios, en orden inverso al de arranque.
func (m *ServiceManager) StopAll() {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	for i := len(m.services) - 1; i >= 0; i-- {
		_ = m.stop(m.services[i])
	}
}

// List devuelve el estado de todos los servicios.
func (m *ServiceManager) List() []Info {
	out := make([]Info, 0, len(m.services))
	for _, s := range m.services {
		info := Info{
			Name:   s.Name(),
			Status: s.Status(),
			Error:  s.LastError(),
			Ports:  s.Ports(),
		}
		if d, ok := s.(Detailer); ok {
			info.Detail = d.Detail()
		}
		out = append(out, info)
	}
	return out
}

// Sites devuelve los proyectos de www/ más las herramientas (Adminer). Escanea
// en el momento (es barato). Los vhosts de Nginx se regeneran con ReloadSites.
func (m *ServiceManager) Sites() ([]vhosts.Site, []vhosts.Skipped, error) {
	return allSites()
}

// ReloadSites regenera los vhosts y, si nginx corre, lo recarga sin cortarlo.
func (m *ServiceManager) ReloadSites() ([]vhosts.Site, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	m.env.Log.Printf("[nginx] recargando sitios...")
	sites, err := m.nginx.ReloadSites()
	if err != nil {
		m.env.Log.Printf("[nginx] error al recargar sitios: %v", err)
	}
	return sites, err
}

// SSLStatus es el estado de HTTPS para la UI.
type SSLStatus struct {
	Enabled   bool   `json:"enabled"`   // HTTPS activado (config.json)
	Available bool   `json:"available"` // mkcert está en bin/mkcert/
	Error     string `json:"error"`     // ej. falta mkcert
}

// SSLStatus dice si HTTPS está activado y si se puede activar.
func (m *ServiceManager) SSLStatus() SSLStatus {
	st := SSLStatus{Enabled: sslEnabled()}
	if _, err := certs.Exe(); err != nil {
		st.Error = err.Error()
	} else {
		st.Available = true
	}
	return st
}

// EnableSSL activa HTTPS:
//  1. Verifica que el 443 esté libre (si no, no cambia nada).
//  2. Instala la CA de mkcert en Windows (aparece un aviso de seguridad).
//  3. Lo guarda en config.json.
//  4. Si Nginx corre, lo reinicia: al arrancar crea los certificados y escucha en el 443.
//
// Se reinicia en vez de `nginx -s reload` porque se agrega un puerto nuevo, y
// en Windows es más seguro que el master lo abra desde cero.
func (m *ServiceManager) EnableSSL() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	if err := procutil.CheckPortFree(config.Current().HTTPSPort); err != nil {
		m.env.Log.Printf("[ssl] no se activa: %v", err)
		return err
	}
	if certs.TrustedByMachine() {
		// La instaló el instalador de Kiltro (como admin): no hace falta el
		// aviso de seguridad de `mkcert -install`.
		m.env.Log.Printf("[ssl] la CA ya está en el almacén de la máquina (instalador)")
	} else {
		m.env.Log.Printf("[ssl] instalando la CA de mkcert (Windows pide confirmación)...")
		if err := certs.Install(m.env.Job); err != nil {
			m.env.Log.Printf("[ssl] %v", err)
			return err
		}
	}
	if err := config.Update(func(s *config.Settings) { s.SSL = true }); err != nil {
		return err
	}
	m.env.Log.Printf("[ssl] HTTPS activado (CA en %s)", certs.CARoot())
	return m.restartNginxForSSL()
}

// DisableSSL vuelve a solo HTTP. La CA queda instalada (desinstalarla será
// una opción de la pantalla de configuración).
func (m *ServiceManager) DisableSSL() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	if err := config.Update(func(s *config.Settings) { s.SSL = false }); err != nil {
		return err
	}
	m.env.Log.Printf("[ssl] HTTPS desactivado")
	return m.restartNginxForSSL()
}

// restartNginxForSSL reinicia Nginx si está corriendo, para que tome el
// cambio de HTTPS. Si está detenido, se aplicará cuando se inicie. Requiere opMu.
func (m *ServiceManager) restartNginxForSSL() error {
	defer m.env.Notify() // la UI muestra los puertos de nginx (80 / 80, 443)
	if st := m.nginx.Status(); st != StatusRunning && st != StatusStarting {
		return nil
	}
	if err := m.stop(m.nginx); err != nil {
		return err
	}
	return m.start(m.nginx)
}

// site busca un sitio de www/ por nombre. La UI manda solo el nombre (nunca
// una ruta): la carpeta sale de nuestra propia lista.
func (m *ServiceManager) site(name string) (vhosts.Site, error) {
	sites, _, err := allSites()
	if err != nil {
		return vhosts.Site{}, err
	}
	for _, s := range sites {
		if s.Name == name && !s.Tool {
			return s, nil
		}
	}
	return vhosts.Site{}, i18n.Err("err.unknownSite", "name", name)
}

// OpenSiteFolder abre la carpeta del proyecto en el Explorador.
func (m *ServiceManager) OpenSiteFolder(name string) error {
	s, err := m.site(name)
	if err != nil {
		return err
	}
	return procutil.ShellOpen(s.Folder, "", "")
}

// OpenSiteTerminal abre una terminal en la carpeta del proyecto: Windows
// Terminal si está instalado, si no PowerShell.
func (m *ServiceManager) OpenSiteTerminal(name string) error {
	s, err := m.site(name)
	if err != nil {
		return err
	}
	if wt, err := exec.LookPath("wt.exe"); err == nil {
		// -d: carpeta inicial. Las comillas protegen rutas con espacios.
		return procutil.ShellOpen(wt, `-d "`+s.Folder+`"`, s.Folder)
	}
	return procutil.ShellOpen("powershell.exe", "-NoExit", s.Folder)
}

// CreateSite crea una carpeta vacía en www/ para un proyecto nuevo y recarga
// los sitios. Devuelve el dominio (ej. "mi-tienda.test").
func (m *ServiceManager) CreateSite(name string) (string, error) {
	n, err := vhosts.CheckNewName(name)
	if err != nil {
		return "", err
	}
	www := config.Current().WWWDir()
	if err := os.MkdirAll(www, 0o755); err != nil {
		return "", err
	}
	dir := filepath.Join(www, n)
	// Comparar también ignorando mayúsculas: Windows trata "Blog" y "blog" como
	// la misma carpeta, y Scan las normalizaría al mismo dominio.
	if entries, err := os.ReadDir(www); err == nil {
		for _, e := range entries {
			if strings.EqualFold(e.Name(), n) || (e.IsDir() && vhosts.NormalizedName(e.Name()) == n) {
				return "", i18n.Err("sites.exists", "path", filepath.Join(www, e.Name()), "domain", n+vhosts.TLD)
			}
		}
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return "", i18n.Wrap(err, "err.create", "path", dir)
	}
	m.env.Log.Printf("[sitios] carpeta creada: %s", dir)
	if _, err := m.ReloadSites(); err != nil {
		return n + vhosts.TLD, err
	}
	return n + vhosts.TLD, nil
}

// HostsStatus es el estado del archivo hosts respecto de los sitios de www/.
type HostsStatus struct {
	UpToDate bool     `json:"upToDate"`
	Domains  []string `json:"domains"` // los que deberían estar
	Error    string   `json:"error"`   // error de lectura o del último intento de actualizar
}

// HostsStatus compara el bloque de Kiltro del hosts con los sitios actuales.
// Solo lee: no pide permisos.
func (m *ServiceManager) HostsStatus() HostsStatus {
	m.hostsMu.Lock()
	lastErr := m.hostsErr
	m.hostsMu.Unlock()

	domains, err := m.siteDomains()
	if err != nil {
		return HostsStatus{Error: err.Error()}
	}
	content, err := hosts.Read()
	if err != nil {
		return HostsStatus{Domains: domains, Error: err.Error()}
	}
	st := HostsStatus{UpToDate: hosts.UpToDate(content, domains), Domains: domains}
	if !st.UpToDate {
		st.Error = lastErr
	}
	return st
}

// SyncHosts deja el bloque de Kiltro del hosts con los dominios de www/.
// Si ya está al día no hace nada (y no aparece el aviso de UAC). Si no, pide
// permisos de admin y espera a que el usuario responda.
func (m *ServiceManager) SyncHosts() error {
	// hostsMu (y no opMu): mientras el usuario mira el aviso de UAC, los
	// botones de iniciar/detener servicios siguen funcionando.
	m.hostsMu.Lock()
	defer m.hostsMu.Unlock()

	domains, err := m.siteDomains()
	if err != nil {
		return err
	}
	content, err := hosts.Read()
	if err != nil {
		m.hostsErr = err.Error()
		return err
	}
	if hosts.UpToDate(content, domains) {
		m.hostsErr = ""
		return nil
	}

	m.env.Log.Printf("[hosts] actualizando bloque KILTRO con %d dominio(s): pidiendo permisos de admin", len(domains))
	if err := hosts.RequestUpdate(domains, config.Path("etc")); err != nil {
		m.hostsErr = err.Error()
		m.env.Log.Printf("[hosts] %v", err)
		return err
	}
	m.hostsErr = ""
	m.env.Log.Printf("[hosts] actualizado (backup en etc\\hosts-backup)")
	return nil
}

func (m *ServiceManager) siteDomains() ([]string, error) {
	sites, _, err := allSites()
	if err != nil {
		return nil, err
	}
	domains := make([]string, 0, len(sites))
	for _, s := range sites {
		domains = append(domains, s.Domain)
	}
	return domains, nil
}

// Close detiene todo de forma limpia y libera recursos. Se llama al cerrar la app.
// Cerrar el Job al final es la red de seguridad: si algo quedó vivo, Windows lo mata.
func (m *ServiceManager) Close() {
	m.StopAll()
	m.env.Log.Printf("Kiltro cerrado")
	m.env.Job.Close()
	m.logFile.Close()
}

// SaveSettings valida y guarda la configuración editable (puertos, red local,
// carpeta de proyectos) y reinicia los servicios afectados que estaban
// corriendo. Devuelve los nombres de los servicios reiniciados.
//
// El orden importa:
//  1. Validar y revisar que los puertos NUEVOS estén libres (si no, no se cambia nada).
//  2. Detener los servicios afectados con la config VIEJA (ej. mysqladmin tiene
//     que hablarle al puerto en el que MySQL está escuchando ahora).
//  3. Guardar config.json.
//  4. Volver a iniciarlos, ya con la config nueva.
func (m *ServiceManager) SaveSettings(next config.Settings) ([]string, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	old, _ := config.Load()
	// Lo que no se edita en esta pantalla se conserva tal cual.
	next.PHP, next.SSL, next.Lang = old.PHP, old.SSL, old.Lang // se cambian con sus propios botones
	if err := next.Validate(); err != nil {
		return nil, err
	}

	// ¿Qué servicios cambian?
	affected := map[string]bool{}
	if next.PHPBasePort != old.PHPBasePort || next.PHPWorkers != old.PHPWorkers {
		affected["php"] = true
		affected["nginx"] = true // el upstream de nginx lista los puertos de PHP
	}
	if next.MySQLPort != old.MySQLPort {
		affected["mysql"] = true
	}
	if next.RedisPort != old.RedisPort {
		affected["redis"] = true
	}
	if next.HTTPPort != old.HTTPPort || next.HTTPSPort != old.HTTPSPort ||
		next.LAN != old.LAN || next.WWWDir() != old.WWWDir() {
		affected["nginx"] = true
	}

	// Servicios afectados que están corriendo (en orden de arranque).
	var running []Service
	for _, s := range m.services {
		if st := s.Status(); affected[s.Name()] && (st == StatusRunning || st == StatusStarting) {
			running = append(running, s)
		}
	}

	// Puertos nuevos que todavía no usa ese mismo servicio: tienen que estar libres.
	newPorts := map[string][]int{
		"php":   next.PHPPorts(),
		"mysql": {next.MySQLPort},
		"redis": {next.RedisPort},
		"nginx": nginxPorts(next),
	}
	for _, s := range running {
		current := map[int]bool{}
		for _, p := range s.Ports() {
			current[p] = true
		}
		for _, p := range newPorts[s.Name()] {
			if current[p] {
				continue
			}
			if err := procutil.CheckPortFree(p); err != nil {
				return nil, i18n.Wrap(err, "settings.notSaved")
			}
		}
	}

	for i := len(running) - 1; i >= 0; i-- {
		if err := m.stop(running[i]); err != nil {
			return nil, err
		}
	}

	if err := config.Update(func(s *config.Settings) {
		php, ssl := s.PHP, s.SSL
		*s = next
		s.PHP, s.SSL = php, ssl
	}); err != nil {
		return nil, err
	}
	m.env.Log.Printf("[config] configuración guardada (http %d, https %d, mysql %d, redis %d, php %d x%d, red local %v, www %s)",
		next.HTTPPort, next.HTTPSPort, next.MySQLPort, next.RedisPort, next.PHPBasePort, next.PHPWorkers, next.LAN, next.WWWDir())

	var restarted []string
	var errs []error
	for _, s := range running {
		if err := m.start(s); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.Name(), err))
			continue
		}
		restarted = append(restarted, s.Name())
	}
	m.env.Notify()
	return restarted, errors.Join(errs...)
}

// UninstallCA quita la CA de Kiltro de los certificados de confianza de
// Windows (mkcert -uninstall; Windows pide confirmación). Si HTTPS estaba
// activado, primero lo desactiva. Los archivos de la CA quedan en etc/ssl/ca:
// si se vuelve a activar HTTPS, se reinstala la misma.
func (m *ServiceManager) UninstallCA() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	if sslEnabled() {
		if err := config.Update(func(s *config.Settings) { s.SSL = false }); err != nil {
			return err
		}
		m.env.Log.Printf("[ssl] HTTPS desactivado (se va a desinstalar la CA)")
		if err := m.restartNginxForSSL(); err != nil {
			return err
		}
	}
	// 1. Almacén de la MÁQUINA (lo puso el instalador): requiere admin, así que
	//    se relanza kiltro.exe --remove-ca con UAC.
	if certs.TrustedByMachine() {
		m.env.Log.Printf("[ssl] quitando la CA del almacén de la máquina (Windows pide permiso de administrador)...")
		if err := removeMachineCAElevated(); err != nil {
			m.env.Log.Printf("[ssl] %v", err)
			return err
		}
		m.env.Log.Printf("[ssl] CA quitada del almacén de la máquina")
	}
	// 2. Almacén del USUARIO (lo puso mkcert -install). Se pregunta DESPUÉS de
	//    quitarla de la máquina: el del usuario también muestra los de la máquina.
	if certs.TrustedByUser() {
		m.env.Log.Printf("[ssl] desinstalando la CA de mkcert (Windows pide confirmación)...")
		if err := certs.Uninstall(m.env.Job); err != nil {
			m.env.Log.Printf("[ssl] %v", err)
			return err
		}
	}
	m.env.Log.Printf("[ssl] CA desinstalada de Windows")
	return nil
}

// removeMachineCAElevated relanza el propio exe como admin para quitar la CA
// del almacén de la máquina (ver main.go, --remove-ca). Igual que con el hosts,
// la ruta del rootCA.pem va como argumento: el proceso elevado no hereda
// KILTRO_ROOT.
func removeMachineCAElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	code, err := procutil.RunElevatedAndWait(exe, `--remove-ca "`+certs.CAFile()+`"`, 2*time.Minute)
	if errors.Is(err, procutil.ErrCancelled) {
		return i18n.Err("ca.removeCancelled")
	}
	if err != nil {
		return err
	}
	if code != 0 || certs.TrustedByMachine() {
		return i18n.Err("ca.removeFailed", "code", code)
	}
	return nil
}
