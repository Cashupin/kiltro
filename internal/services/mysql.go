package services

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
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

const (
	mysqlInitTimeout  = 2 * time.Minute  // inicialización de la primera vez
	mysqlStartTimeout = 60 * time.Second // puede tardar si tiene que recuperarse de un corte
	mysqlStopTimeout  = 60 * time.Second // tiempo para que InnoDB guarde todo en disco

	// Usuario interno que Kiltro crea al inicializar, SOLO con permiso de
	// SHUTDOWN. Sirve para apagar MySQL de forma limpia aunque el usuario le
	// ponga contraseña a root. Su contraseña (aleatoria) queda en
	// etc/mysql/kiltro-admin.cnf.
	mysqlAdminUser = "kiltro_admin"
)

// MySQL implementa Service.
//
// REGLA DE ORO: nunca matar mysqld a la fuerza salvo como último recurso.
// Matarlo mientras escribe puede corromper las bases de datos.
type MySQL struct {
	env *Env

	mu      sync.Mutex
	status  Status
	lastErr string
	detail  string
	cmd     *exec.Cmd
	exited  chan struct{}

	// port es el puerto con el que se ARRANCÓ mysqld. Para apagarlo se usa este
	// (no el de config.json): si el usuario cambió el puerto mientras corría,
	// mysqladmin tiene que hablarle al puerto viejo.
	port int
}

func NewMySQL(env *Env) *MySQL {
	return &MySQL{env: env, status: StatusStopped}
}

func (m *MySQL) Name() string { return "mysql" }
// Ports: mientras corre, el puerto con que arrancó; detenido, el de config.json.
func (m *MySQL) Ports() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status != StatusStopped && m.status != StatusError && m.port != 0 {
		return []int{m.port}
	}
	return []int{config.Current().MySQLPort}
}

// runningPort es el puerto del mysqld lanzado por Kiltro (para apagarlo).
func (m *MySQL) runningPort() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.port != 0 {
		return m.port
	}
	return config.Current().MySQLPort
}

func (m *MySQL) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *MySQL) LastError() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr
}

func (m *MySQL) Detail() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.detail
}

func (m *MySQL) set(s Status, errMsg, detail string) {
	m.mu.Lock()
	m.status, m.lastErr, m.detail = s, errMsg, detail
	m.mu.Unlock()
	m.env.Notify()
}

func (m *MySQL) fail(err error) error {
	m.set(StatusError, err.Error(), "")
	return err
}

// Rutas que usa MySQL (todas relativas a la raíz de la app).
func mysqlDataDir() string  { return config.Path("data", "mysql") }
func mysqlIni() string      { return config.Path("etc", "mysql", "my.ini") }
func mysqlAdminCnf() string { return config.Path("etc", "mysql", "kiltro-admin.cnf") }
func mysqlErrorLog() string { return config.Path("logs", "mysql", "error.log") }

// Start prepara my.ini, inicializa la base si es la primera vez y lanza mysqld.
func (m *MySQL) Start() error {
	m.set(StatusStarting, "", "")

	bin, err := mysqlBinDir()
	if err != nil {
		return m.fail(err)
	}
	for _, dir := range []string{config.Path("etc", "mysql"), config.Path("logs", "mysql")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return m.fail(err)
		}
	}
	port := config.Current().MySQLPort
	m.mu.Lock()
	m.port = port
	m.mu.Unlock()
	if err := writeMyIni(filepath.Dir(bin), port); err != nil {
		return m.fail(err)
	}

	// ¿Primera vez? (regla 6 de CLAUDE.md)
	empty, err := dirIsEmpty(mysqlDataDir())
	if err != nil {
		return m.fail(err)
	}
	if empty {
		m.set(StatusStarting, "", "Inicializando (primera vez)…")
		if err := m.initialize(bin); err != nil {
			return m.fail(err)
		}
	} else if !looksLikeMySQLDataDir(mysqlDataDir()) {
		// Tiene archivos pero no es una base de MySQL: no tocamos nada.
		return m.fail(fmt.Errorf("%s tiene archivos pero no parece una carpeta de datos de MySQL; "+
			"no se inicializa para no borrar nada. Vacíala o muévela a mano", mysqlDataDir()))
	}

	m.set(StatusStarting, "", "")

	// --defaults-file tiene que ser el PRIMER argumento. Además hace que mysqld
	// lea SOLO nuestro my.ini e ignore otros que haya en la PC (C:\my.ini, C:\Windows\my.ini).
	cmd := exec.Command(filepath.Join(bin, "mysqld.exe"), "--defaults-file="+mysqlIni())
	cmd.Dir = config.Path()
	// La salida de consola de mysqld se descarta: todo lo importante va a
	// logs/mysql/error.log (y la consola trae avisos que confunden, ej. plugins
	// opcionales que faltan en algunas copias de MySQL).

	// Recordamos hasta dónde llegaba el log, para que si se cae se muestren
	// solo los errores de ESTE arranque y no de arranques anteriores.
	logOffset := fileSize(mysqlErrorLog())

	if err := procutil.Start(m.env.Job, cmd); err != nil {
		return m.fail(err)
	}
	exited := make(chan struct{})
	m.mu.Lock()
	m.cmd, m.exited = cmd, exited
	m.mu.Unlock()
	go m.watch(cmd, exited, logOffset)

	deadline := time.Now().Add(mysqlStartTimeout)
	for {
		select {
		case <-exited:
			return errors.New(m.LastError())
		default:
		}
		if _, ok := procutil.ListenerPID(m.runningPort()); ok {
			m.set(StatusRunning, "", "")
			m.env.Log.Printf("[mysql] escuchando en 127.0.0.1:%d (PID %d)", m.runningPort(), cmd.Process.Pid)
			return nil
		}
		if time.Now().After(deadline) {
			// No llegó a estar listo: se intenta el apagado limpio igual.
			_ = m.Stop()
			return m.fail(fmt.Errorf("MySQL no empezó a escuchar en el puerto %d tras %s; revisa %s",
				m.runningPort(), mysqlStartTimeout, mysqlErrorLog()))
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// initialize crea la carpeta de datos con `mysqld --initialize-insecure`
// (root@localhost sin contraseña) y el usuario interno kiltro_admin.
func (m *MySQL) initialize(bin string) error {
	m.env.Log.Printf("[mysql] data\\mysql vacía: inicializando (puede tardar un poco)...")

	// Contraseña aleatoria para kiltro_admin. Se guarda ANTES de inicializar:
	// si se perdiera, no podríamos apagar con ese usuario.
	pw, err := randomPassword()
	if err != nil {
		return err
	}
	cnf := fmt.Sprintf("# Generado por Kiltro. Usuario interno solo para apagar MySQL (permiso SHUTDOWN).\n"+
		"[client]\nuser=%s\npassword=%s\nhost=127.0.0.1\nport=%d\n", mysqlAdminUser, pw, m.runningPort())
	if err := os.WriteFile(mysqlAdminCnf(), []byte(cnf), 0o600); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", mysqlAdminCnf(), err)
	}

	// --init-file: SQL que mysqld ejecuta durante la inicialización.
	initSQL := filepath.Join(config.Path("etc", "mysql"), "init.sql")
	sql := fmt.Sprintf("CREATE USER '%[1]s'@'localhost' IDENTIFIED BY '%[2]s';\n"+
		"CREATE USER '%[1]s'@'127.0.0.1' IDENTIFIED BY '%[2]s';\n"+
		"GRANT SHUTDOWN ON *.* TO '%[1]s'@'localhost';\n"+
		"GRANT SHUTDOWN ON *.* TO '%[1]s'@'127.0.0.1';\n", mysqlAdminUser, pw)
	if err := os.WriteFile(initSQL, []byte(sql), 0o600); err != nil {
		return err
	}
	defer os.Remove(initSQL) // tiene la contraseña: no dejarlo tirado

	if err := os.MkdirAll(mysqlDataDir(), 0o755); err != nil {
		return err
	}

	cmd := exec.Command(filepath.Join(bin, "mysqld.exe"),
		"--defaults-file="+mysqlIni(),
		"--initialize-insecure",
		"--init-file="+initSQL,
	)
	cmd.Dir = config.Path()
	initOffset := fileSize(mysqlErrorLog())
	out, err := runAndWait(m.env.Job, cmd, mysqlInitTimeout)
	if err != nil {
		// La carpeta estaba vacía antes de empezar, así que lo que haya ahora es
		// una inicialización a medias (sin datos del usuario). Se limpia para que
		// el próximo intento pueda inicializar de nuevo.
		_ = os.RemoveAll(mysqlDataDir())
		return fmt.Errorf("falló la inicialización de MySQL: %v %s %s", err, out, lastErrorLines(mysqlErrorLog(), initOffset, 5))
	}

	m.env.Log.Printf("[mysql] inicializado: usuario root@localhost sin contraseña")
	return nil
}

// watch espera a que mysqld termine y actualiza el estado.
//
// Ojo: en Windows, MySQL 8 arranca DOS procesos: el que lanzamos es un
// "monitor", y ese lanza al servidor real (el que tiene los datos y el puerto).
// Si el monitor muere de golpe (ej. lo matan desde el Administrador de tareas),
// el servidor puede quedar huérfano. A diferencia de nginx, NO se lo mata
// directo: primero se le pide el apagado limpio.
func (m *MySQL) watch(cmd *exec.Cmd, exited chan struct{}, logOffset int64) {
	waitErr := cmd.Wait()

	m.mu.Lock()
	expected := m.status == StatusStopping
	m.mu.Unlock()
	if !expected {
		m.stopOrphan()
	}

	m.mu.Lock()
	if expected {
		m.status, m.lastErr = StatusStopped, ""
	} else {
		msg := "MySQL terminó inesperadamente"
		if waitErr != nil {
			msg += " (" + waitErr.Error() + ")"
		}
		if tail := lastErrorLines(mysqlErrorLog(), logOffset, 3); tail != "" {
			msg += ": " + tail
		} else {
			msg += "; revisa " + mysqlErrorLog()
		}
		m.status, m.lastErr = StatusError, msg
	}
	m.detail = ""
	m.cmd, m.exited = nil, nil
	m.mu.Unlock()

	close(exited)
	if !expected {
		m.env.Log.Printf("[mysql] %s", m.LastError())
	}
	m.env.Notify()
}

// Stop apaga MySQL de forma limpia con `mysqladmin shutdown` y espera a que
// mysqld termine de guardar todo. Solo si no termina en mysqlStopTimeout se
// mata el proceso (último recurso: puede corromper datos).
func (m *MySQL) Stop() error {
	m.mu.Lock()
	cmd, exited := m.cmd, m.exited
	if cmd == nil {
		m.mu.Unlock()
		return nil
	}
	m.status = StatusStopping
	m.mu.Unlock()
	m.env.Notify()

	if err := m.mysqladminShutdown(); err != nil {
		m.env.Log.Printf("[mysql] ADVERTENCIA: no se pudo pedir el apagado limpio: %v", err)
	}

	select {
	case <-exited:
		return nil
	case <-time.After(mysqlStopTimeout):
	}

	m.env.Log.Printf("[mysql] ADVERTENCIA: no se detuvo en %s; terminando a la fuerza (PID %d). "+
		"Revisa tus bases de datos.", mysqlStopTimeout, cmd.Process.Pid)
	procutil.KillTree(cmd.Process.Pid)
	select {
	case <-exited:
		return nil
	case <-time.After(5 * time.Second):
		return m.fail(fmt.Errorf("no se pudo detener MySQL (PID %d)", cmd.Process.Pid))
	}
}

// stopOrphan apaga de forma limpia un servidor mysqld que quedó vivo después de
// que su proceso monitor murió. Solo si no termina en mysqlStopTimeout, se mata.
func (m *MySQL) stopOrphan() {
	pid, ok := procutil.ListenerPID(m.runningPort())
	if !ok || !strings.EqualFold(procutil.ProcessName(pid), "mysqld.exe") {
		return // no quedó nada escuchando: no hay huérfano
	}
	m.env.Log.Printf("[mysql] el proceso monitor terminó pero el servidor (PID %d) sigue vivo; apagándolo de forma limpia", pid)
	if err := m.mysqladminShutdown(); err != nil {
		m.env.Log.Printf("[mysql] ADVERTENCIA: no se pudo pedir el apagado limpio: %v", err)
	}
	deadline := time.Now().Add(mysqlStopTimeout)
	for time.Now().Before(deadline) {
		if _, ok := procutil.ListenerPID(m.runningPort()); !ok {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	m.env.Log.Printf("[mysql] ADVERTENCIA: el servidor huérfano no se detuvo en %s; terminando a la fuerza (PID %d)", mysqlStopTimeout, pid)
	procutil.KillTree(pid)
}

// mysqladminShutdown pide el apagado. Primero con el usuario interno
// kiltro_admin; si no existe (base creada antes de Kiltro), con root sin contraseña.
func (m *MySQL) mysqladminShutdown() error {
	bin, err := mysqlBinDir()
	if err != nil {
		return err
	}
	admin := filepath.Join(bin, "mysqladmin.exe")

	var attempts [][]string
	if _, err := os.Stat(mysqlAdminCnf()); err == nil {
		attempts = append(attempts, // -P va DESPUÉS de --defaults-file: pisa el puerto guardado en el .cnf
		// (que es el de cuando se inicializó MySQL).
		[]string{"--defaults-file=" + mysqlAdminCnf(), fmt.Sprintf("-P%d", m.runningPort()), "shutdown"})
	}
	attempts = append(attempts, []string{
		"--no-defaults", "-u", "root", "-h", "127.0.0.1", fmt.Sprintf("-P%d", m.runningPort()), "shutdown",
	})

	var errs []string
	for _, args := range attempts {
		cmd := exec.Command(admin, args...)
		out, err := runAndWait(m.env.Job, cmd, 30*time.Second)
		if err == nil {
			return nil
		}
		errs = append(errs, strings.TrimSpace(out))
	}
	return errors.New(strings.Join(errs, " / "))
}

// --- helpers ---

// mysqlBinDir busca bin/mysql/mysql-*/bin (donde está mysqld.exe).
func mysqlBinDir() (string, error) {
	matches, _ := filepath.Glob(config.Path("bin", "mysql", "mysql-*", "bin", "mysqld.exe"))
	if len(matches) == 0 {
		return "", fmt.Errorf("no se encontró MySQL: se esperaba %s",
			config.Path("bin", "mysql", "mysql-<versión>", "bin", "mysqld.exe"))
	}
	sort.Strings(matches)
	return filepath.Dir(matches[len(matches)-1]), nil
}

// writeMyIni genera etc/mysql/my.ini (se sobrescribe en cada arranque).
// MySQL acepta "/" en las rutas en Windows.
func writeMyIni(baseDir string, port int) error {
	slash := filepath.ToSlash
	ini := fmt.Sprintf(`# Generado por Kiltro. NO EDITAR: se sobrescribe en cada arranque.
[mysqld]
basedir="%s"
datadir="%s"
log-error="%s"
port=%d
# Solo accesible desde esta PC.
bind-address=127.0.0.1

# Ajustes para gastar poca RAM (entorno de desarrollo):
# performance_schema es lo que más ahorra (100-400 MB).
performance_schema=OFF
# Protocolo X (puerto 33060): no se usa.
mysqlx=OFF
innodb_buffer_pool_size=128M
max_connections=50
`, slash(baseDir), slash(mysqlDataDir()), slash(mysqlErrorLog()), port)

	if err := os.WriteFile(mysqlIni(), []byte(ini), 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", mysqlIni(), err)
	}
	return nil
}

// dirIsEmpty: true si la carpeta no existe o no tiene nada.
func dirIsEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

// looksLikeMySQLDataDir revisa que haya archivos típicos de una carpeta de datos de MySQL 8.
func looksLikeMySQLDataDir(dir string) bool {
	for _, name := range []string{"mysql.ibd", "ibdata1"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

// lastErrorLines devuelve las últimas n líneas con "[ERROR]" del log de MySQL,
// mirando solo lo escrito a partir del byte offset (ver fileSize).
func lastErrorLines(logPath string, offset int64, n int) string {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	if offset > 0 && offset <= int64(len(data)) {
		data = data[offset:]
	}
	var errs []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "[ERROR]") {
			errs = append(errs, strings.TrimSpace(line))
		}
	}
	if len(errs) > n {
		errs = errs[len(errs)-n:]
	}
	return strings.Join(errs, "\n")
}

// fileSize devuelve el tamaño de un archivo (0 si no existe).
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// runAndWait ejecuta un comando corto dentro del Job y espera (con timeout).
func runAndWait(job *procutil.Job, cmd *exec.Cmd, timeout time.Duration) (string, error) {
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
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return "timeout", fmt.Errorf("timeout tras %s", timeout)
	}
}

func randomPassword() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
