// Package services define los servicios que Kiltro levanta (nginx, php, mysql, redis)
// y el ServiceManager que los coordina.
package services

// Status es el estado de un servicio, tal como se muestra en la UI.
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusStopping Status = "stopping"
	StatusError    Status = "error" // se cayó solo o no pudo arrancar; ver LastError()
)

// Service es lo que tiene que implementar cada servicio (nginx.go, php.go, ...).
//
// Las reglas comunes (chequear puertos, loguear, avisar a la UI) las hace el
// ServiceManager; cada servicio solo sabe cómo arrancarse y detenerse.
type Service interface {
	// Name es el identificador corto: "nginx", "php", "mysql"...
	Name() string

	// Ports son los puertos TCP que el servicio necesita libres para arrancar.
	Ports() []int

	// Start lanza el servicio. Debe usar procutil.Start para que quede dentro
	// del Job Object y sin ventana.
	Start() error

	// Stop detiene el servicio de forma limpia (ej. `nginx -s quit`). Solo si
	// eso falla tras un timeout, termina el proceso a la fuerza.
	Stop() error

	// Status devuelve el estado actual.
	Status() Status

	// LastError devuelve el último error (vacío si no hubo).
	LastError() string
}

// Detailer es opcional: un servicio que lo implementa muestra un texto extra
// en la UI (ej. PHP: "4/4 workers").
type Detailer interface {
	Detail() string
}
