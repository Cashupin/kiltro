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

	"kiltro/internal/config"
	"kiltro/internal/procutil"
)

// Redis para Windows: Redis no publica versiones oficiales para Windows. Se usa
// el port de tporadowski, 5.0.14.1 (el más usado en Windows). Ya no se
// actualiza, pero es estable para desarrollo.

const (
	redisStartTimeout = 10 * time.Second
	// Al apagarse, Redis guarda los datos en data/redis/dump.rdb. Con bases
	// grandes puede tardar unos segundos.
	redisStopTimeout = 30 * time.Second
)

// Rutas de Redis (relativas a la raíz de la app).
func redisConf() string    { return config.Path("etc", "redis", "redis.conf") }
func redisDataDir() string { return config.Path("data", "redis") }
func redisLog() string     { return config.Path("logs", "redis", "redis.log") }

// Redis implementa Service.
type Redis struct {
	env *Env

	mu      sync.Mutex
	status  Status
	lastErr string
	cmd     *exec.Cmd
	exited  chan struct{}
	port    int // puerto con el que arrancó (para apagarlo aunque cambie la config)
}

func NewRedis(env *Env) *Redis {
	return &Redis{env: env, status: StatusStopped}
}

func (r *Redis) Name() string { return "redis" }

// Ports: mientras corre, el puerto con que arrancó; detenido, el de config.json.
func (r *Redis) Ports() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.status != StatusStopped && r.status != StatusError && r.port != 0 {
		return []int{r.port}
	}
	return []int{config.Current().RedisPort}
}

func (r *Redis) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *Redis) LastError() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastErr
}

func (r *Redis) set(s Status, errMsg string) {
	r.mu.Lock()
	r.status, r.lastErr = s, errMsg
	r.mu.Unlock()
	r.env.Notify()
}

func (r *Redis) fail(err error) error {
	r.set(StatusError, err.Error())
	return err
}

// Start genera redis.conf y lanza redis-server. Vuelve cuando ya escucha.
func (r *Redis) Start() error {
	r.set(StatusStarting, "")

	bin, err := redisBinDir()
	if err != nil {
		return r.fail(err)
	}
	for _, dir := range []string{config.Path("etc", "redis"), redisDataDir(), config.Path("logs", "redis")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return r.fail(err)
		}
	}
	port := config.Current().RedisPort
	if err := writeRedisConf(port); err != nil {
		return r.fail(err)
	}
	r.mu.Lock()
	r.port = port
	r.mu.Unlock()

	cmd := exec.Command(filepath.Join(bin, "redis-server.exe"), redisConf())
	cmd.Dir = config.Path()
	// Lo normal va a logs/redis/redis.log. Pero si redis.conf tiene un error,
	// Redis lo avisa por consola ANTES de abrir el log: por eso se guarda.
	var console bytes.Buffer
	cmd.Stdout = &console
	cmd.Stderr = &console

	if err := procutil.Start(r.env.Job, cmd); err != nil {
		return r.fail(err)
	}
	exited := make(chan struct{})
	r.mu.Lock()
	r.cmd, r.exited = cmd, exited
	r.mu.Unlock()
	go r.watch(cmd, exited, &console)

	deadline := time.Now().Add(redisStartTimeout)
	for {
		select {
		case <-exited:
			return errors.New(r.LastError())
		default:
		}
		if _, ok := procutil.ListenerPID(port); ok {
			r.set(StatusRunning, "")
			r.env.Log.Printf("[redis] escuchando en 127.0.0.1:%d (PID %d)", port, cmd.Process.Pid)
			return nil
		}
		if time.Now().After(deadline) {
			_ = r.Stop()
			return r.fail(fmt.Errorf("Redis no empezó a escuchar en el puerto %d tras %s; revisa %s",
				port, redisStartTimeout, redisLog()))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// watch espera a que redis-server termine y actualiza el estado.
func (r *Redis) watch(cmd *exec.Cmd, exited chan struct{}, console *bytes.Buffer) {
	waitErr := cmd.Wait()

	r.mu.Lock()
	expected := r.status == StatusStopping
	if expected {
		r.status, r.lastErr = StatusStopped, ""
	} else {
		msg := "Redis terminó inesperadamente"
		if waitErr != nil {
			msg += " (" + waitErr.Error() + ")"
		}
		if out := strings.TrimSpace(console.String()); out != "" {
			msg += ": " + lastLines(out, 3)
		} else {
			msg += "; revisa " + redisLog()
		}
		r.status, r.lastErr = StatusError, msg
	}
	r.cmd, r.exited = nil, nil
	r.mu.Unlock()

	close(exited)
	if !expected {
		r.env.Log.Printf("[redis] %s", r.LastError())
	}
	r.env.Notify()
}

// Stop apaga Redis de forma limpia con `redis-cli shutdown`: Redis guarda los
// datos en dump.rdb y se cierra solo. Solo si no termina en redisStopTimeout
// se mata el proceso (se perderían los cambios desde el último guardado).
func (r *Redis) Stop() error {
	r.mu.Lock()
	cmd, exited, port := r.cmd, r.exited, r.port
	if cmd == nil {
		r.mu.Unlock()
		return nil
	}
	r.status = StatusStopping
	r.mu.Unlock()
	r.env.Notify()

	if bin, err := redisBinDir(); err == nil {
		cli := exec.Command(filepath.Join(bin, "redis-cli.exe"), "-h", "127.0.0.1", "-p", fmt.Sprint(port), "shutdown")
		if out, err := runAndWait(r.env.Job, cli, 10*time.Second); err != nil {
			r.env.Log.Printf("[redis] ADVERTENCIA: no se pudo pedir el apagado limpio: %v %s", err, out)
		}
	}

	select {
	case <-exited:
		return nil
	case <-time.After(redisStopTimeout):
	}

	r.env.Log.Printf("[redis] ADVERTENCIA: no se detuvo en %s; terminando a la fuerza (PID %d)", redisStopTimeout, cmd.Process.Pid)
	procutil.KillTree(cmd.Process.Pid)
	select {
	case <-exited:
		return nil
	case <-time.After(5 * time.Second):
		return r.fail(fmt.Errorf("no se pudo detener Redis (PID %d)", cmd.Process.Pid))
	}
}

// redisBinDir busca bin/redis/<versión>/redis-server.exe (o directo en bin/redis/).
func redisBinDir() (string, error) {
	matches, _ := filepath.Glob(config.Path("bin", "redis", "*", "redis-server.exe"))
	if direct := config.Path("bin", "redis", "redis-server.exe"); fileExists(direct) {
		matches = append(matches, direct)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no se encontró Redis: se esperaba %s",
			config.Path("bin", "redis", "redis-<versión>", "redis-server.exe"))
	}
	sort.Strings(matches)
	return filepath.Dir(matches[len(matches)-1]), nil
}

// writeRedisConf genera etc/redis/redis.conf (se sobrescribe en cada arranque).
func writeRedisConf(port int) error {
	slash := filepath.ToSlash
	conf := fmt.Sprintf(`# Generado por Kiltro. NO EDITAR: se sobrescribe en cada arranque.
# Solo esta PC: ningún otro equipo de la red puede conectarse.
bind 127.0.0.1
protected-mode yes
port %d

# Datos: se guardan en dump.rdb cada tanto y al apagar (redis-cli shutdown).
dir "%s"
dbfilename dump.rdb
save 900 1
save 300 10
save 60 10000

logfile "%s"
loglevel notice
databases 16
`, port, slash(redisDataDir()), slash(redisLog()))
	if err := os.WriteFile(redisConf(), []byte(conf), 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", redisConf(), err)
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// lastLines devuelve las últimas n líneas no vacías de un texto.
func lastLines(text string, n int) string {
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
