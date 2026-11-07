package services

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"

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
		return nil, fmt.Errorf("no se pudo crear la carpeta logs: %w", err)
	}
	f, err := os.OpenFile(config.Path("logs", "kiltro.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir logs/kiltro.log: %w", err)
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
		// Orden de arranque: PHP y MySQL antes que Nginx (Nginx es la "puerta"
		// de entrada; cuando atiende, lo demás ya está listo).
		// StopAll los detiene en orden inverso.
		services: []Service{
			NewPHP(env),
			NewMySQL(env),
			nginx,
		},
	}
	logger.Printf("Kiltro iniciado. Raíz: %s", config.Path())
	return m, nil
}

// get busca un servicio por nombre.
func (m *ServiceManager) get(name string) (Service, error) {
	for _, s := range m.services {
		if s.Name() == name {
			return s, nil
		}
	}
	return nil, fmt.Errorf("servicio desconocido: %q", name)
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
