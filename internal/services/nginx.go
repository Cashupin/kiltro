package services

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"kiltro/internal/certs"
	"kiltro/internal/config"
	"kiltro/internal/procutil"
	"kiltro/internal/vhosts"
)

const (
	nginxPort         = 80               // fijo por ahora; configurable en la etapa 8
	nginxSSLPort      = 443              // HTTPS, solo si está activado (ver sslEnabled)
	nginxStartTimeout = 5 * time.Second  // cuánto esperar a que empiece a escuchar
	nginxStopTimeout  = 10 * time.Second // cuánto esperar al apagado limpio antes de forzar
)

// Rutas relativas a la raíz de la app. Nginx las resuelve contra el "prefix" (-p),
// que es justamente la raíz, así que en nginx.conf se pueden usar relativas.
const (
	nginxConfRel  = "etc/nginx/nginx.conf"
	nginxErrorLog = "logs/nginx/error.log"
)

// Nginx implementa Service.
type Nginx struct {
	env *Env

	mu      sync.Mutex // protege los campos de abajo
	status  Status
	lastErr string
	cmd     *exec.Cmd     // proceso master de nginx mientras corre; nil si no
	exited  chan struct{} // se cierra cuando el master termina
}

// NewNginx crea el servicio (no lo arranca).
func NewNginx(env *Env) *Nginx {
	return &Nginx{env: env, status: StatusStopped}
}

func (n *Nginx) Name() string { return "nginx" }

// Ports incluye el 443 solo si HTTPS está activado (si no, no se toca).
func (n *Nginx) Ports() []int {
	if sslEnabled() {
		return []int{nginxPort, nginxSSLPort}
	}
	return []int{nginxPort}
}

// sslEnabled dice si el usuario activó HTTPS (config.json).
func sslEnabled() bool {
	cfg, _ := config.Load()
	return cfg.SSL
}

func (n *Nginx) Status() Status {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.status
}

func (n *Nginx) LastError() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.lastErr
}

func (n *Nginx) setStatus(s Status, errMsg string) {
	n.mu.Lock()
	n.status = s
	n.lastErr = errMsg
	n.mu.Unlock()
	n.env.Notify()
}

// fail marca el servicio en error y devuelve el mismo error (para usar con return).
func (n *Nginx) fail(err error) error {
	n.setStatus(StatusError, err.Error())
	return err
}

// Start genera nginx.conf, lo valida con `nginx -t` y lanza nginx.
// Vuelve cuando nginx ya está escuchando en el puerto (o con error).
func (n *Nginx) Start() error {
	n.setStatus(StatusStarting, "")

	exe, err := nginxExe()
	if err != nil {
		return n.fail(err)
	}
	if err := writeNginxConf(exe); err != nil {
		return n.fail(err)
	}
	if _, err := n.generateSites(exe); err != nil {
		return n.fail(err)
	}

	// Validar la config antes de arrancar: da errores mucho más claros.
	if out, err := runNginxCommand(n.env.Job, exe, "-t"); err != nil {
		return n.fail(fmt.Errorf("la configuración de nginx tiene errores: %s", out))
	}

	// Lanzar el proceso master. Él mismo lanza a sus workers; como el master
	// está en el Job, los workers también quedan dentro.
	cmd := exec.Command(exe, nginxArgs()...)
	cmd.Dir = config.Path()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr // nginx escribe aquí los errores de arranque (ej. bind() falló)

	if err := procutil.Start(n.env.Job, cmd); err != nil {
		return n.fail(err)
	}

	exited := make(chan struct{})
	n.mu.Lock()
	n.cmd = cmd
	n.exited = exited
	n.mu.Unlock()

	go n.watch(cmd, exited, &stderr)

	// Esperar a que el puerto esté escuchando. ListenerPID solo mira la tabla
	// TCP, no abre el puerto, así que no interfiere con nginx.
	deadline := time.Now().Add(nginxStartTimeout)
	for {
		select {
		case <-exited:
			// Murió durante el arranque; watch() ya guardó el motivo.
			return errors.New(n.LastError())
		default:
		}

		if _, ok := procutil.ListenerPID(nginxPort); ok {
			n.setStatus(StatusRunning, "")
			n.env.Log.Printf("[nginx] escuchando en el puerto %d (PID %d)", nginxPort, cmd.Process.Pid)
			return nil
		}

		if time.Now().After(deadline) {
			procutil.KillTree(cmd.Process.Pid)
			<-exited
			return n.fail(fmt.Errorf("nginx no empezó a escuchar en el puerto %d tras %s", nginxPort, nginxStartTimeout))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// watch espera a que el proceso master termine y actualiza el estado.
// Si terminó sin que se lo pidiéramos, es una caída: se marca como error.
func (n *Nginx) watch(cmd *exec.Cmd, exited chan struct{}, stderr *bytes.Buffer) {
	waitErr := cmd.Wait() // después de Wait es seguro leer stderr

	// Si el master murió de golpe (crash o lo mataron desde fuera), sus workers
	// NO mueren solos en Windows y siguen ocupando el puerto 80. Los limpiamos.
	// En un apagado limpio ya no queda ninguno y esto no hace nada.
	procutil.KillTree(cmd.Process.Pid)

	n.mu.Lock()
	expected := n.status == StatusStopping
	if expected {
		n.status = StatusStopped
		n.lastErr = ""
	} else {
		msg := "nginx terminó inesperadamente"
		if waitErr != nil {
			msg += " (" + waitErr.Error() + ")"
		}
		if out := strings.TrimSpace(stderr.String()); out != "" {
			msg += ": " + out
		}
		n.status = StatusError
		n.lastErr = msg
	}
	n.cmd = nil
	n.exited = nil
	n.mu.Unlock()

	close(exited)
	if !expected {
		n.env.Log.Printf("[nginx] %s", n.LastError())
	}
	n.env.Notify()
}

// Stop detiene nginx de forma limpia con `nginx -s quit` (termina de atender
// las peticiones en curso y sale). Si no sale tras nginxStopTimeout, se fuerza.
func (n *Nginx) Stop() error {
	n.mu.Lock()
	cmd, exited := n.cmd, n.exited
	if cmd == nil {
		n.mu.Unlock()
		return nil
	}
	n.status = StatusStopping
	n.mu.Unlock()
	n.env.Notify()

	quitOK := false
	if exe, err := nginxExe(); err == nil {
		// `-s quit` lee el PID desde el archivo pid de nginx.conf, por eso
		// se le pasan los mismos -p/-c/-e que al arrancar.
		if out, err := runNginxCommand(n.env.Job, exe, "-s", "quit"); err != nil {
			n.env.Log.Printf("[nginx] `nginx -s quit` falló: %s", out)
		} else {
			quitOK = true
		}
	}

	if quitOK {
		select {
		case <-exited:
			return nil
		case <-time.After(nginxStopTimeout):
			n.env.Log.Printf("[nginx] no se detuvo en %s", nginxStopTimeout)
		}
	}

	// Último recurso: matar master y workers.
	n.env.Log.Printf("[nginx] terminando a la fuerza (PID %d)", cmd.Process.Pid)
	procutil.KillTree(cmd.Process.Pid)
	select {
	case <-exited:
		return nil
	case <-time.After(3 * time.Second):
		return n.fail(fmt.Errorf("no se pudo detener nginx (PID %d)", cmd.Process.Pid))
	}
}

// ReloadSites vuelve a escanear www/ y regenera los vhosts. Si nginx está
// corriendo, valida la config y la aplica con `nginx -s reload`, que recarga sin
// cortar las conexiones en curso. Si está detenido, solo regenera los archivos.
func (n *Nginx) ReloadSites() ([]vhosts.Site, error) {
	exe, err := nginxExe()
	if err != nil {
		return nil, err
	}
	// nginx.conf también se regenera: si es la primera vez, todavía no tiene el
	// include de sites/.
	if err := writeNginxConf(exe); err != nil {
		return nil, err
	}
	sites, err := n.generateSites(exe)
	if err != nil {
		return nil, err
	}

	if n.Status() != StatusRunning {
		return sites, nil
	}

	if out, err := runNginxCommand(n.env.Job, exe, "-t"); err != nil {
		return sites, fmt.Errorf("la configuración de nginx tiene errores (se sigue usando la anterior): %s", out)
	}
	if out, err := runNginxCommand(n.env.Job, exe, "-s", "reload"); err != nil {
		return sites, fmt.Errorf("`nginx -s reload` falló: %s", out)
	}
	n.env.Log.Printf("[nginx] recargado con %d sitio(s)", len(sites))
	return sites, nil
}

// generateSites escanea www/ y escribe un vhost por proyecto en etc/nginx/sites/.
func (n *Nginx) generateSites(exe string) ([]vhosts.Site, error) {
	sites, skipped, err := allSites()
	if err != nil {
		return nil, err
	}
	for _, s := range skipped {
		n.env.Log.Printf("[nginx] carpeta www\\%s ignorada: %s", s.Folder, s.Reason)
	}

	opts := vhosts.Options{
		SitesDir:    config.Path("etc", "nginx", "sites"),
		FastcgiConf: filepath.Join(filepath.Dir(exe), "conf", "fastcgi.conf"),
		Port:        nginxPort,
	}

	// HTTPS: asegurar un certificado por dominio (más "localhost", que usa el
	// server por defecto en el 443). Solo se crean los que faltan.
	if sslEnabled() {
		domains := []string{"localhost"}
		for _, s := range sites {
			domains = append(domains, s.Domain)
		}
		created, err := certs.Ensure(n.env.Job, domains)
		if len(created) > 0 {
			n.env.Log.Printf("[ssl] certificados creados: %s", strings.Join(created, ", "))
		}
		if err != nil {
			return nil, err
		}
		opts.SSLPort = nginxSSLPort
		opts.SSLCert = certs.Files
	}

	err = vhosts.Generate(sites, opts)
	if err != nil {
		return nil, err
	}
	return sites, nil
}

// --- helpers ---

// nginxExe busca bin/nginx/nginx-*/nginx.exe. Si hay varias versiones, usa la
// "mayor" por orden alfabético (elegir versión desde la config vendrá después).
func nginxExe() (string, error) {
	matches, _ := filepath.Glob(config.Path("bin", "nginx", "nginx-*", "nginx.exe"))
	if len(matches) == 0 {
		return "", fmt.Errorf("no se encontró nginx: se esperaba %s",
			config.Path("bin", "nginx", "nginx-<versión>", "nginx.exe"))
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

// nginxArgs son los argumentos comunes a todas las llamadas a nginx.
//   - -p: "prefix", la carpeta contra la que se resuelven las rutas relativas.
//   - -c: archivo de configuración.
//   - -e: log de errores que se usa ANTES de leer nginx.conf (si no, nginx
//     intentaría escribir en logs/error.log por defecto).
func nginxArgs(extra ...string) []string {
	prefix := filepath.ToSlash(config.Path()) + "/"
	args := []string{"-p", prefix, "-c", nginxConfRel, "-e", nginxErrorLog}
	return append(args, extra...)
}

// runNginxCommand ejecuta un comando corto de nginx (-t, -s quit) y espera a que
// termine. Devuelve su salida (nginx escribe estos mensajes en stderr).
func runNginxCommand(job *procutil.Job, exe string, extra ...string) (string, error) {
	cmd := exec.Command(exe, nginxArgs(extra...)...)
	cmd.Dir = config.Path()
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := procutil.Start(job, cmd); err != nil {
		return err.Error(), err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return strings.TrimSpace(out.String()), err
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		return "timeout", errors.New("timeout ejecutando nginx " + strings.Join(extra, " "))
	}
}

// phpUpstream arma el bloque `upstream` con un `server` por worker de PHP
// (los mismos puertos que levanta php.go). Nginx reparte las peticiones entre ellos.
func phpUpstream() string {
	var b strings.Builder
	b.WriteString("    upstream php_workers {\n")
	for i := 0; i < phpWorkers; i++ {
		fmt.Fprintf(&b, "        server 127.0.0.1:%d;\n", phpBasePort+i)
	}
	b.WriteString("    }\n")
	return b.String()
}

// defaultSSL son las líneas de HTTPS del server por defecto (vacío si HTTPS
// está apagado). Usa el certificado de "localhost"; un dominio sin vhost que
// entre por https verá una advertencia de certificado, lo cual es correcto.
func defaultSSL() string {
	if !sslEnabled() {
		return ""
	}
	cert, key := certs.Files("localhost")
	return fmt.Sprintf(`
        listen       %d ssl default_server;
        ssl_certificate      "%s";
        ssl_certificate_key  "%s";`, nginxSSLPort, filepath.ToSlash(cert), filepath.ToSlash(key))
}

// writeNginxConf genera etc/nginx/nginx.conf. Por ahora es una config fija:
// un server en localhost:80 que sirve etc/nginx/default/ (un phpinfo de prueba)
// pasando los .php a los workers de PHP. En la etapa 3 esto pasa a generarse
// desde templates/ con los vhosts de www/.
func writeNginxConf(exe string) error {
	defaultRoot := config.Path("etc", "nginx", "default")

	// Carpetas que nginx necesita que existan (no crea las carpetas padre).
	for _, dir := range []string{
		config.Path("etc", "nginx", "temp"),
		config.Path("logs", "nginx"),
		defaultRoot,
		config.Path("etc", "nginx", "sites"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("no se pudo crear %s: %w", dir, err)
		}
	}

	// Página de prueba. Vive fuera de www/ a propósito: www/ es solo para proyectos.
	index := filepath.Join(defaultRoot, "index.php")
	if err := os.WriteFile(index, []byte("<?php\n// Generado por Kiltro (página de prueba).\nphpinfo();\n"), 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", index, err)
	}

	// mime.types y fastcgi.conf vienen con nginx; se usan directo desde su carpeta.
	// Nginx acepta "/" como separador en Windows, y las comillas protegen espacios.
	nginxConfDir := filepath.Join(filepath.Dir(exe), "conf")
	mimeTypes := filepath.ToSlash(filepath.Join(nginxConfDir, "mime.types"))
	fastcgiConf := filepath.ToSlash(filepath.Join(nginxConfDir, "fastcgi.conf"))

	conf := fmt.Sprintf(`# Generado por Kiltro. NO EDITAR: se sobrescribe en cada arranque.
worker_processes  1;
error_log  "%[1]s" warn;
pid        "etc/nginx/nginx.pid";

events {
    worker_connections  1024;
}

http {
    include       "%[2]s";
    default_type  application/octet-stream;
    access_log    "logs/nginx/access.log";

    client_body_temp_path  "etc/nginx/temp/client_body";
    proxy_temp_path        "etc/nginx/temp/proxy";
    fastcgi_temp_path      "etc/nginx/temp/fastcgi";
    uwsgi_temp_path        "etc/nginx/temp/uwsgi";
    scgi_temp_path         "etc/nginx/temp/scgi";

%[4]s
    # Server por defecto: atiende localhost y cualquier dominio que no tenga
    # vhost propio (muestra el phpinfo de prueba).
    server {
        listen       %[3]d default_server;%[8]s
        server_name  localhost;
        root         "%[5]s";
        index        index.php index.html;

        location / {
            try_files $uri $uri/ =404;
        }

        location ~ \.php$ {
            # Si el .php no existe, 404 (sin esto, PHP intentaría adivinar el script).
            try_files $uri =404;
            include       "%[6]s";
            fastcgi_pass  php_workers;
        }
    }

    # Un archivo por proyecto de www/ (los genera internal/vhosts).
    include "%[7]s/*.conf";
}
`, nginxErrorLog, mimeTypes, nginxPort, phpUpstream(), filepath.ToSlash(defaultRoot), fastcgiConf,
		filepath.ToSlash(config.Path("etc", "nginx", "sites")), defaultSSL())

	path := config.Path(filepath.FromSlash(nginxConfRel))
	if err := os.WriteFile(path, []byte(conf), 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", path, err)
	}
	return nil
}
