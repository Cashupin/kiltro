// Package tray es el ícono de Kiltro en la bandeja del sistema (junto al reloj),
// con un menú para abrir la ventana e iniciar/detener servicios.
//
// Usa fyne.io/systray. Este paquete no sabe nada de Wails ni del ServiceManager:
// recibe funciones (Callbacks) y el estado de los servicios (Update). Así se
// puede leer y probar por separado.
//
// # Cómo convive con Wails (lo no obvio)
//
// En Windows cada ventana recibe sus mensajes (clics, menú...) SOLO en el hilo
// del sistema operativo que la creó, a través de un "loop de mensajes".
// Wails ya tiene su loop en el hilo principal. systray crea su propia ventana
// oculta y necesita su propio loop; por eso lo corremos en una goroutine que
// se "pega" a un hilo con runtime.LockOSThread(). Sin eso, Go podría mover la
// goroutine a otro hilo y el ícono dejaría de responder.
package tray

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"fyne.io/systray"
)

// Service es lo que el menú necesita saber de cada servicio.
type Service struct {
	Name   string // id interno, ej. "php"
	Label  string // lo que se ve, ej. "PHP"
	Status string // "stopped", "starting", "running", "stopping", "error"
}

// Callbacks son las acciones del menú. Se llaman desde la goroutine de la
// bandeja: si tardan (iniciar MySQL puede tardar segundos), no pasa nada,
// porque cada clic se atiende en su propia goroutine.
type Callbacks struct {
	Open     func()            // mostrar la ventana de Kiltro
	Toggle   func(name string) // iniciar/detener un servicio
	StartAll func()
	StopAll  func()
	Quit     func() // cerrar Kiltro de verdad
}

var (
	mu      sync.Mutex
	ready   bool                             // el menú ya se creó
	items   = map[string]*systray.MenuItem{} // un ítem por servicio
	current []Service                        // último estado recibido
	exited  = make(chan struct{})            // se cierra cuando el ícono ya se quitó
	started bool
)

// Start muestra el ícono en la bandeja. services es la lista inicial (define
// qué ítems tiene el menú y en qué orden). icon es un .ico.
func Start(icon []byte, services []Service, cb Callbacks) {
	mu.Lock()
	current = services
	started = true
	mu.Unlock()

	onReady := func() {
		systray.SetIcon(icon)
		systray.SetTitle("Kiltro")

		// Clic izquierdo en el ícono = abrir la ventana. El derecho muestra el menú.
		systray.SetOnTapped(cb.Open)

		open := systray.AddMenuItem("Abrir Kiltro", "Muestra la ventana de Kiltro")
		systray.AddSeparator()

		mu.Lock()
		for _, s := range current {
			// Checkbox: el tick indica "corriendo". Al hacer clic se alterna.
			items[s.Name] = systray.AddMenuItemCheckbox(s.Label, "Iniciar/detener "+s.Label, false)
		}
		mu.Unlock()

		systray.AddSeparator()
		startAll := systray.AddMenuItem("Iniciar todo", "Inicia PHP, MySQL y Nginx")
		stopAll := systray.AddMenuItem("Detener todo", "Detiene todos los servicios")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Salir", "Detiene los servicios y cierra Kiltro")

		// Cada ítem avisa sus clics por un canal (ClickedCh). Una goroutine por
		// ítem espera esos clics y llama a la acción correspondiente.
		onClick(open, cb.Open)
		for name, item := range items {
			name := name
			onClick(item, func() { cb.Toggle(name) })
		}
		onClick(startAll, cb.StartAll)
		onClick(stopAll, cb.StopAll)
		onClick(quit, cb.Quit)

		mu.Lock()
		ready = true
		apply()
		mu.Unlock()
	}

	onExit := func() { close(exited) }

	go func() {
		runtime.LockOSThread() // ver el comentario del paquete
		systray.Run(onReady, onExit)
	}()
}

// onClick llama a f (en otra goroutine) cada vez que se hace clic en el ítem.
func onClick(item *systray.MenuItem, f func()) {
	go func() {
		for range item.ClickedCh {
			go f()
		}
	}()
}

// Update refleja el estado de los servicios en el menú y en el tooltip.
// Se puede llamar desde cualquier goroutine, incluso antes de que el menú exista.
func Update(services []Service) {
	mu.Lock()
	defer mu.Unlock()
	current = services
	if ready {
		apply()
	}
}

// apply actualiza ticks, textos y tooltip con `current`. Requiere mu tomado.
func apply() {
	running := 0
	for _, s := range current {
		item, ok := items[s.Name]
		if !ok {
			continue
		}
		title := s.Label
		switch s.Status {
		case "running":
			running++
			item.Check()
		case "starting":
			title += " (iniciando…)"
			item.Check()
		case "stopping":
			title += " (deteniendo…)"
			item.Uncheck()
		case "error":
			title += " (error)"
			item.Uncheck()
		default:
			item.Uncheck()
		}
		item.SetTitle(title)
	}
	systray.SetTooltip(fmt.Sprintf("Kiltro — %d/%d servicios corriendo", running, len(current)))
}

// Stop quita el ícono de la bandeja. Se llama al cerrar la app; si no, el
// ícono queda "fantasma" hasta que pases el mouse por encima.
func Stop() {
	mu.Lock()
	wasStarted := started
	mu.Unlock()
	if !wasStarted {
		return
	}
	systray.Quit()
	select {
	case <-exited:
	case <-time.After(2 * time.Second): // no bloquear el cierre si algo falló
	}
}
