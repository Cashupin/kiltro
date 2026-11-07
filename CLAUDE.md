# Kiltro — entorno de desarrollo local liviano para Windows

> "Kiltro" es nombre provisional. Si cambia, actualizar este archivo.

## Qué es

App de escritorio para Windows que reemplaza a Laragon: levanta servicios portables
(Nginx, PHP, MySQL, Redis), genera vhosts `*.test` automáticamente por cada carpeta en `www/`,
gestiona el archivo hosts, certificados SSL locales y permite cambiar la versión de PHP.

Prioridad número uno: **liviana**. Debe correr bien en PCs con poca RAM. Nada de Docker,
nada de Electron, nada de servicios de Windows instalados.

## Stack (no cambiar sin preguntar)

- **Wails v2** (NO v3, que está en alpha y tiene otra API). Si dudas de una API, es la de v2.
- **Go** 1.22+ para todo el backend.
- **Frontend:** Svelte + TypeScript, plantilla oficial `svelte-ts` de Wails v2. CSS simple, sin frameworks pesados.
- Usar la versión de Svelte que trae la plantilla de Wails v2 (hoy Svelte 5), con sintaxis clásica:
  **sin runes** (`let`, `$:`, `on:click`, `export let`).
- **Plataforma:** solo Windows 10/11. No hace falta soporte para macOS/Linux.
- Evitar CGO. No agregar dependencias nuevas sin preguntar primero y explicar por qué.

## Estructura del código

```
kiltro/
├── main.go                  # arranque de Wails
├── app.go                   # métodos expuestos al frontend (bindings)
├── internal/
│   ├── services/            # ServiceManager + un archivo por servicio
│   │   ├── manager.go
│   │   ├── service.go       # interface Service
│   │   ├── nginx.go
│   │   ├── php.go
│   │   ├── mysql.go
│   │   └── redis.go
│   ├── procutil/            # Job Object, ocultar ventanas, chequeo de puertos
│   ├── vhosts/              # escaneo de www/ y generación de configs
│   ├── hosts/               # lectura/escritura del archivo hosts
│   ├── certs/               # integración con mkcert
│   └── config/              # config.json de la app
├── templates/
│   └── nginx/vhost.conf.tmpl
└── frontend/                # Svelte
```

## Estructura en runtime (relativa al .exe, todo portable)

```
<carpeta de la app>/
├── kiltro.exe
├── config.json
├── bin/
│   ├── nginx/nginx-1.x.x/
│   ├── php/php-8.3.x/       # puede haber varias versiones
│   ├── mysql/mysql-8.x.x/
│   ├── redis/
│   └── mkcert/
├── etc/
│   ├── nginx/sites/         # vhosts generados
│   └── ssl/
├── www/                     # un proyecto por carpeta → <carpeta>.test
├── data/mysql/
└── logs/
```

Todas las rutas se resuelven relativas al ejecutable, nunca rutas absolutas hardcodeadas.

- La raíz de la app se resuelve en un solo lugar (`internal/config`, función `AppRoot()`):
  1. Si existe la variable de entorno `KILTRO_ROOT`, usar esa (para `wails dev`).
  2. Si no, la carpeta del ejecutable (`os.Executable()` + `filepath.Dir`).
  Ningún otro paquete calcula rutas por su cuenta; todos usan `AppRoot()`.
- En desarrollo, la carpeta runtime vive en `kiltro/runtime/` (ignorada en git) y se
  apunta con `KILTRO_ROOT`.
- `bin/`, `data/`, `logs/`, `etc/` y `www/` NO van al repo (.gitignore). Los binarios se
  descargan a mano por ahora; documentar en README qué versión va en qué carpeta.

## Reglas críticas (leer antes de tocar procesos)

1. **Procesos hijos dentro de un Job Object** (`golang.org/x/sys/windows`) con
   `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, para que si la app se cae no queden procesos zombie.
2. **Lanzar procesos sin ventana de consola** (`CREATE_NO_WINDOW` / `HideWindow`).
3. **Antes de arrancar un servicio, verificar que el puerto esté libre.** Si está ocupado,
   mostrar un error claro diciendo qué puerto y, si se puede, qué proceso lo usa.
4. **Detener servicios de forma limpia:**
   - Nginx: `nginx -s quit -p <prefix>`.
   - MySQL: `mysqladmin shutdown`. **Nunca** matar `mysqld` a la fuerza salvo como último
     recurso tras un timeout, porque corrompe datos.
   - Solo si el apagado limpio falla tras un timeout, terminar el proceso.
5. **PHP con `php-cgi.exe` (FastCGI).** En Windows no existe PHP-FPM.
   - Lanzar varios workers (por defecto 4) en puertos consecutivos desde 9000, con un
     `upstream` en Nginx.
   - `php-cgi` se cierra solo tras `PHP_FCGI_MAX_REQUESTS` requests: setear
     `PHP_FCGI_MAX_REQUESTS=0` en el entorno **y** además supervisar y relanzar workers caídos.
6. **MySQL primera vez:** si `data/mysql` está vacío, inicializar con
   `mysqld --initialize-insecure --datadir=...` antes del primer arranque.
7. **Archivo hosts:**
   - Solo editar dentro de un bloque marcado `# KILTRO START` / `# KILTRO END`. Nunca tocar
     líneas fuera del bloque.
   - Requiere admin. La app principal **no** corre como admin; la escritura del hosts se hace
     relanzando el propio exe con un flag (ej. `kiltro.exe --write-hosts <archivo>`) vía `runas`.
   - Hacer backup del hosts antes de modificarlo.
8. **Dominio local:** `.test` (nunca `.dev` ni `.local`).
9. Toda acción de servicios se loguea en `logs/kiltro.log` con timestamp.

## Puntos conocidos a resolver en su etapa

- **System tray:** Wails v2 no trae bandeja nativa. En la etapa correspondiente evaluar una
  librería (ej. `fyne.io/systray`) y cómo convive con el loop de Wails. Proponer antes de implementar.
- **Cambio de versión de PHP:** cada versión con su propio `php.ini`. Las extensiones (DLL)
  deben corresponder a esa versión exacta.

## Plan por etapas

Trabajar **una etapa a la vez**. No adelantar trabajo de etapas siguientes.

- [x] **Etapa 1:** Proyecto Wails v2 inicializado. `ServiceManager` + interface `Service`.
      Nginx arranca/se detiene desde un botón. Job Object y chequeo de puerto funcionando.
  - [x] 1a: `wails init` (svelte-ts) + `config.AppRoot()` + carpeta `runtime/` para dev.
  - [x] 1b: `procutil`: Job Object, lanzar sin ventana, chequeo de puerto.
  - [x] 1c: interface `Service`, `ServiceManager`, Nginx start/stop desde un botón.
- [x] **Etapa 2:** PHP-CGI con workers + supervisión. Un vhost fijo de prueba que sirve un `phpinfo()`.
- [x] **Etapa 3:** Escaneo de `www/`, generación automática de vhosts desde plantilla,
      bloque en el archivo hosts con elevación. Detección de proyectos Laravel (root en `/public`).
- [x] **Etapa 4:** MySQL (inicialización, arranque, apagado limpio).
  - [x] 4a: MySQL: inicialización, arranque dentro del Job, apagado limpio con `mysqladmin`.
  - [x] 4b: Gestor de BD con Adminer (un archivo PHP en `bin/adminer/`) servido en
        `http://adminer.test`, solo accesible desde 127.0.0.1, con botón en la UI.
- [x] **Etapa 5:** Selector de versión de PHP.
- [ ] **Etapa 6:** System tray (minimizar a bandeja, start/stop desde el menú).
- [ ] **Etapa 7:** SSL con mkcert (`https://proyecto.test`).
- [ ] **Etapa 8:** Redis, visor de logs, pantalla de configuración (puertos, carpeta www, etc.).
- [ ] **Etapa 9:** Pulido de la UI y del flujo de uso.
  - Ocultar las herramientas internas (`adminer.test`) de la lista de "Sitios": se usan desde
    su botón, no como un proyecto más (siguen en Nginx y en el hosts).
  - "Gestor BD" siempre disponible: si al pulsarlo faltan PHP, MySQL o Nginx, arrancarlos
    (en ese orden) y recién ahí abrir Adminer.
  - Mejorar el aspecto general de la ventana de Kiltro.

## Forma de trabajar

- Antes de cambios grandes o de estructura, explicar el plan y esperar confirmación.
- Después de cada cambio, compilar (`go build ./...` y/o `wails build`) y corregir errores antes de terminar.
- Código simple y legible por sobre código "elegante". El dueño viene de PHP/Laravel y tiene
  que poder leer y depurar el Go.
- Comentar las partes no obvias, sobre todo manejo de procesos, Job Objects y elevación.
- Al terminar una tarea: resumen corto de lo hecho, cómo probarlo, y un mensaje de commit sugerido.
- Al completar una etapa, marcarla con [x] en este archivo y actualizar "Estado actual".

## Estado actual

**Etapas 1 a 5 completas** (2026-11-06). Siguiente: Etapa 6 (system tray: proponer librería antes de implementar).

Notas para las próximas etapas:
- La plantilla svelte-ts de Wails v2.17.1 trae Svelte 5: se usa `mount()` en `main.ts`, pero
  los componentes siguen con sintaxis clásica (sin runes). Vite fijado en `^7` por un conflicto
  de peer deps de la plantilla.
- Nginx probado con 1.30.4 en `bin/nginx/nginx-1.30.4/`. `nginx.conf` se genera en cada arranque
  (`etc/nginx/nginx.conf`). Los vhosts salen de `templates/nginx/vhost.conf.tmpl` (embebida en
  el exe) hacia `etc/nginx/sites/`; "Recargar sitios" valida con `-t` y aplica con `-s reload`.
- Si el master de un servicio muere de golpe, sus hijos NO mueren solos en Windows: usar
  `procutil.KillTree` al detectar la salida (ver `nginx.go`, `watch`).
- PHP probado con 8.4.27 NTS (por defecto, la más nueva) y 8.3.33 TS (de Laragon). Versión elegida
  en `config.json` (`internal/config/config.go`, por ahora solo el campo `php`); el orden es
  numérico (`phpversions.go`). Cambiar de versión reinicia solo PHP, Nginx no se toca.
  `php.ini` vive junto a cada versión y Kiltro solo lo crea si falta (desde `php.ini-development`);
  `extension_dir` se pasa con `-d` al arrancar. 4 workers en 9000-9003 detrás de `upstream php_workers`.
- La página de prueba (phpinfo) está en `etc/nginx/default/`, fuera de `www/` a propósito.
  Es el `default_server`: cualquier dominio sin vhost cae ahí.
- Hosts: `internal/hosts`. El proceso elevado (`kiltro.exe --write-hosts <archivo>`) NO usa
  `AppRoot()` ni `KILTRO_ROOT` (no hereda el entorno): recibe todo por argumento y revalida que
  cada dominio sea `*.test`. Solo `127.0.0.1` (sin `::1`, Nginx escucha solo en IPv4).
  Backups en `etc/hosts-backup/`.
- MySQL probado con 8.4.3. `my.ini` se genera en `etc/mysql/`. En Windows `mysqld` arranca un
  proceso monitor + el servidor: si el monitor muere, el servidor huérfano se apaga con
  `mysqladmin` (`stopOrphan`). Usuario interno `kiltro_admin` (solo SHUTDOWN), contraseña en
  `etc/mysql/kiltro-admin.cnf`; `root` sin contraseña.
- Adminer 6.3.0: `etc/adminer/index.php` es un envoltorio generado (permite root sin
  contraseña) y el vhost es `LocalOnly` (solo 127.0.0.1). Tema propio en `templates/adminer/`
  (en modo oscuro Adminer no carga `adminer.css`: `adminer-dark.css` se arma uniendo ambos).
  `adminer` es nombre reservado en `www/`.
