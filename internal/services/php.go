package services

import (
	"bytes"
	"errors"
	"fmt"
	"kiltro/internal/i18n"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"kiltro/internal/config"
	"kiltro/internal/procutil"
)

// En Windows no existe PHP-FPM. Se usa php-cgi.exe en modo FastCGI (-b puerto):
// cada php-cgi es UN proceso que atiende UNA petición a la vez. Para atender
// varias en paralelo se lanzan varios "workers", cada uno en su puerto, y Nginx
// reparte entre ellos con un bloque `upstream` (ver nginx.go).
const (
	phpStartTimeout = 10 * time.Second

	// Supervisión: si un worker se cae, se relanza tras phpRestartDelay.
	// Si un mismo worker se cae más de phpMaxCrashes veces en phpCrashWindow,
	// algo está mal (ej. php.ini roto) y se deja de insistir.
	phpRestartDelay = 1 * time.Second
	phpMaxCrashes   = 5
	phpCrashWindow  = 30 * time.Second
)

// Extensiones que se activan al crear el php.ini por primera vez (lo típico para Laravel).
var phpDefaultExtensions = []string{
	"curl", "fileinfo", "gd", "intl", "mbstring",
	"openssl", "pdo_mysql", "pdo_sqlite", "sqlite3", "zip",
}

// phpWorker es un php-cgi escuchando en un puerto.
type phpWorker struct {
	port    int
	cmd     *exec.Cmd   // proceso actual; nil si está caído/relanzándose
	crashes []time.Time // caídas recientes (para detectar bucles de caídas)
}

// PHP implementa Service: un grupo de workers php-cgi supervisados.
type PHP struct {
	env *Env

	mu       sync.Mutex // protege todo lo de abajo
	status   Status
	lastErr  string
	workers  []*phpWorker
	stopping bool   // true mientras/después de Stop: los supervisores NO relanzan
	exe      string // php-cgi.exe en uso
	ini      string // php.ini en uso

	supervisors sync.WaitGroup // una goroutine por worker
}

// NewPHP crea el servicio (no lo arranca).
func NewPHP(env *Env) *PHP {
	return &PHP{env: env, status: StatusStopped}
}

func (p *PHP) Name() string { return "php" }

// Ports: mientras corre, los puertos de los workers lanzados; detenido, los de
// config.json (cantidad y puerto inicial se configuran en la pantalla de configuración).
func (p *PHP) Ports() []int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.status != StatusStopped && p.status != StatusError && len(p.workers) > 0 {
		ports := make([]int, len(p.workers))
		for i, w := range p.workers {
			ports[i] = w.port
		}
		return ports
	}
	return config.Current().PHPPorts()
}

func (p *PHP) Status() Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

func (p *PHP) LastError() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastErr
}

// Detail muestra cuántos workers están vivos, ej. "4/4 workers".
func (p *PHP) Detail() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.status != StatusRunning && p.status != StatusStarting {
		return ""
	}
	alive := 0
	for _, w := range p.workers {
		if w.cmd != nil {
			alive++
		}
	}
	return i18n.T("php.workers", "alive", alive, "total", len(p.workers))
}

func (p *PHP) setStatus(s Status, errMsg string) {
	p.mu.Lock()
	p.status = s
	p.lastErr = errMsg
	p.mu.Unlock()
	p.env.Notify()
}

func (p *PHP) fail(err error) error {
	p.setStatus(StatusError, err.Error())
	return err
}

// Start prepara php.ini, lanza los workers y espera a que todos escuchen.
func (p *PHP) Start() error {
	// Si quedaban supervisores de una ejecución anterior (ej. tras un error),
	// esperamos a que terminen antes de empezar de nuevo.
	p.supervisors.Wait()

	p.setStatus(StatusStarting, "")

	exe, warning, err := phpCgiExe()
	if err != nil {
		return p.fail(err)
	}
	if warning != "" {
		p.env.Log.Printf("[php] ADVERTENCIA: %s", warning)
	}
	ini, err := ensurePhpIni(filepath.Dir(exe))
	if err != nil {
		return p.fail(err)
	}

	p.mu.Lock()
	p.exe, p.ini = exe, ini
	p.stopping = false
	p.workers = nil
	for _, port := range config.Current().PHPPorts() {
		p.workers = append(p.workers, &phpWorker{port: port})
	}
	workers := p.workers
	p.mu.Unlock()

	for _, w := range workers {
		cmd, stderr, err := p.launch(w)
		if err != nil {
			p.killAll()
			return p.fail(err)
		}
		p.supervisors.Add(1)
		go p.supervise(w, cmd, stderr)
	}

	// Esperar a que todos los puertos estén escuchando.
	deadline := time.Now().Add(phpStartTimeout)
	for {
		if p.Status() == StatusError { // un supervisor se rindió durante el arranque
			return errors.New(p.LastError())
		}
		if p.allListening() {
			p.setStatus(StatusRunning, "")
			ports := p.Ports()
			p.env.Log.Printf("[php] %d workers escuchando en %d-%d (%s)",
				len(ports), ports[0], ports[len(ports)-1], exe)
			return nil
		}
		if time.Now().After(deadline) {
			p.killAll()
			return p.fail(i18n.Err("php.notListening", "timeout", phpStartTimeout))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (p *PHP) allListening() bool {
	for _, port := range p.Ports() {
		if _, ok := procutil.ListenerPID(port); !ok {
			return false
		}
	}
	return true
}

// launch lanza un php-cgi para el worker. Se hace con el mutex tomado para que
// no pueda colarse un relanzamiento justo mientras Stop() está matando workers.
func (p *PHP) launch(w *phpWorker) (*exec.Cmd, *bytes.Buffer, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopping {
		return nil, nil, i18n.Err("php.stopped")
	}

	extDir := filepath.Join(filepath.Dir(p.exe), "ext")
	cmd := exec.Command(p.exe,
		"-b", fmt.Sprintf("127.0.0.1:%d", w.port), // modo FastCGI en ese puerto
		"-c", p.ini, // php.ini de esta versión
		// extension_dir se pasa aquí (ruta absoluta) en vez de escribirlo en
		// php.ini, para que la carpeta siga funcionando aunque se mueva de lugar.
		"-d", "extension_dir="+extDir,
	)
	cmd.Dir = filepath.Dir(p.exe)
	// php-cgi se cierra solo tras PHP_FCGI_MAX_REQUESTS peticiones (500 por
	// defecto). 0 = sin límite. Igual se supervisa por si se cae por otro motivo.
	cmd.Env = append(os.Environ(), "PHP_FCGI_MAX_REQUESTS=0")
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr

	if err := procutil.Start(p.env.Job, cmd); err != nil {
		return nil, nil, i18n.Wrap(err, "php.worker", "port", w.port)
	}
	w.cmd = cmd
	return cmd, &stderr, nil
}

// supervise espera a que el worker termine. Si no fue a pedido (Stop), lo
// relanza. Hay una goroutine de estas por worker mientras el servicio corre.
func (p *PHP) supervise(w *phpWorker, cmd *exec.Cmd, out *bytes.Buffer) {
	defer p.supervisors.Done()

	for {
		waitErr := cmd.Wait()

		p.mu.Lock()
		w.cmd = nil
		if p.stopping {
			p.mu.Unlock()
			return
		}
		// Es una caída: registrarla y ver si estamos en un bucle de caídas.
		now := time.Now()
		recent := w.crashes[:0]
		for _, t := range w.crashes {
			if now.Sub(t) < phpCrashWindow {
				recent = append(recent, t)
			}
		}
		w.crashes = append(recent, now)
		tooMany := len(w.crashes) > phpMaxCrashes
		p.mu.Unlock()
		p.env.Notify() // cambió la cantidad de workers vivos

		msg := i18n.T("php.workerDied", "port", w.port)
		if waitErr != nil {
			msg += " (" + waitErr.Error() + ")"
		}
		if s := strings.TrimSpace(out.String()); s != "" {
			msg += ": " + s
		}

		if tooMany {
			msg = i18n.T("php.tooManyCrashes", "msg", msg, "n", phpMaxCrashes, "window", phpCrashWindow)
			p.env.Log.Printf("[php] %s", msg)
			p.killAll()
			p.setStatus(StatusError, msg)
			return
		}
		p.env.Log.Printf("[php] %s; relanzando en %s", msg, phpRestartDelay)

		// Relanzar (con reintentos si el propio lanzamiento falla).
		for {
			time.Sleep(phpRestartDelay)
			newCmd, newOut, err := p.launch(w)
			if err == nil {
				cmd, out = newCmd, newOut
				p.env.Log.Printf("[php] worker :%d relanzado (PID %d)", w.port, cmd.Process.Pid)
				p.env.Notify()
				break
			}
			p.mu.Lock()
			stopping := p.stopping
			p.mu.Unlock()
			if stopping {
				return
			}
			p.env.Log.Printf("[php] no se pudo relanzar el worker :%d: %v", w.port, err)
		}
	}
}

// killAll marca el servicio como "deteniéndose" (para que nadie relance) y
// termina todos los workers vivos.
func (p *PHP) killAll() {
	p.mu.Lock()
	p.stopping = true
	var pids []int
	for _, w := range p.workers {
		if w.cmd != nil {
			pids = append(pids, w.cmd.Process.Pid)
		}
	}
	p.mu.Unlock()

	for _, pid := range pids {
		procutil.KillTree(pid)
	}
}

// Stop detiene todos los workers.
//
// php-cgi no tiene un comando de apagado limpio como nginx o mysql, pero
// tampoco guarda datos propios: terminarlo es seguro. Lo importante es marcar
// `stopping` ANTES, para que los supervisores no lo vuelvan a lanzar.
func (p *PHP) Stop() error {
	p.mu.Lock()
	p.status = StatusStopping
	p.mu.Unlock()
	p.env.Notify()

	p.killAll()

	// Esperar a que todos los supervisores terminen (= todos los procesos murieron).
	done := make(chan struct{})
	go func() { p.supervisors.Wait(); close(done) }()
	select {
	case <-done:
		p.setStatus(StatusStopped, "")
		return nil
	case <-time.After(5 * time.Second):
		return p.fail(i18n.Err("php.workersStuck"))
	}
}

// --- helpers ---

// phpCgiExe (qué versión usar) está en phpversions.go.

// ensurePhpIni devuelve la ruta del php.ini de esa versión de PHP. Si no existe,
// lo crea copiando php.ini-development y activando phpDefaultExtensions.
// Si ya existe NO lo toca: es del usuario.
func ensurePhpIni(phpDir string) (string, error) {
	ini := filepath.Join(phpDir, "php.ini")
	if _, err := os.Stat(ini); err == nil {
		return ini, nil
	}

	src := filepath.Join(phpDir, "php.ini-development")
	data, err := os.ReadFile(src)
	if err != nil {
		return "", i18n.Wrap(err, "php.noIniSource", "src", src)
	}
	text := string(data)

	// Descomentar ";extension=xxx" → "extension=xxx".
	for _, ext := range phpDefaultExtensions {
		re := regexp.MustCompile(`(?m)^;extension=` + regexp.QuoteMeta(ext) + `\s*$`)
		text = re.ReplaceAllString(text, "extension="+ext)
	}

	header := "; Creado por Kiltro a partir de php.ini-development.\n" +
		"; Kiltro no vuelve a modificar este archivo: puedes editarlo libremente.\n" +
		"; (extension_dir lo pasa Kiltro al arrancar, no hace falta definirlo aquí)\n\n"

	if err := os.WriteFile(ini, []byte(header+text), 0o644); err != nil {
		return "", i18n.Wrap(err, "err.create", "path", ini)
	}
	return ini, nil
}
