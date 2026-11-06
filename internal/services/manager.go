package services

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"kiltro/internal/config"
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
		// Orden de arranque: PHP antes que Nginx (Nginx le pasa los .php).
		// StopAll los detiene en orden inverso.
		services: []Service{
			NewPHP(env),
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
	if st := s.Status(); st == StatusRunning || st == StatusStarting {
		return nil // ya está corriendo
	}

	for _, port := range s.Ports() {
		if err := procutil.CheckPortFree(port); err != nil {
			m.env.Log.Printf("[%s] no se inicia: %v", name, err)
			return err
		}
	}

	m.env.Log.Printf("[%s] iniciando...", name)
	if err := s.Start(); err != nil {
		m.env.Log.Printf("[%s] error al iniciar: %v", name, err)
		return err
	}
	m.env.Log.Printf("[%s] iniciado", name)
	return nil
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

// Sites devuelve los proyectos de www/ (escanea en el momento, es barato).
// No toca archivos: para regenerar los vhosts está ReloadSites.
func (m *ServiceManager) Sites() ([]vhosts.Site, []vhosts.Skipped, error) {
	return vhosts.Scan(config.Path("www"))
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

// Close detiene todo de forma limpia y libera recursos. Se llama al cerrar la app.
// Cerrar el Job al final es la red de seguridad: si algo quedó vivo, Windows lo mata.
func (m *ServiceManager) Close() {
	m.StopAll()
	m.env.Log.Printf("Kiltro cerrado")
	m.env.Job.Close()
	m.logFile.Close()
}
