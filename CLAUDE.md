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
│   ├── tray/                # ícono y menú de la bandeja (fyne.io/systray)
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

- **System tray:** resuelto en la etapa 6 con `fyne.io/systray` (ver `internal/tray`).
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
- [x] **Etapa 6:** System tray (minimizar a bandeja, start/stop desde el menú).
- [x] **Etapa 7:** SSL con mkcert (`https://proyecto.test`).
- [x] **Etapa 8:** Redis, visor de logs, pantalla de configuración (puertos, carpeta www, etc.).
  - [x] 8a: Pantalla de configuración (puertos, workers de PHP, carpeta de proyectos) en
        `config.json`; Nginx solo en `127.0.0.1` con opción "Accesible desde la red local";
        botón para desinstalar la CA de mkcert.
  - [x] 8b: Visor de logs (pestaña Logs: últimas líneas, actualización automática, copiar,
        abrir carpeta, vaciar).
  - [x] 8c: Redis (5.0.14.1 de Laragon), `redis.conf` generado, apagado con `redis-cli shutdown`.
- [x] **Etapa 9:** Pulido de la UI y del flujo de uso.
  - [x] 9a: Flujo de uso.
    - Ocultar las herramientas internas (`adminer.test`) de la lista de "Sitios": se usan desde
      su botón, no como un proyecto más (siguen en Nginx y en el hosts).
    - "Gestor BD" siempre disponible: si al pulsarlo faltan PHP, MySQL o Nginx, arrancarlos
      (en ese orden) y recién ahí abrir Adminer.
    - Por sitio: botones "Abrir", "Carpeta" (Explorador) y "Terminal" (PowerShell en la carpeta).
    - "Nuevo sitio": crea una carpeta vacía en `www/` (con nombre válido) y recarga los sitios.
  - [x] 9b: Aspecto de la ventana (CSS simple, sin frameworks).
    - Estado con punto de color, botones ▶/■, "Iniciar todo" / "Detener todo" arriba.
    - Barra superior con el nombre; la ruta raíz pasa a Configuración.
    - Hosts y HTTPS agrupados en un bloque; paleta con acento azul (como el tema de Adminer).
  - [x] 9c: Ícono propio de Kiltro (SVG → `.ico` con varios tamaños; exe y bandeja; hoy es la
        "W" de la plantilla de Wails).
  - [x] 9d: `php` y `composer` en la terminal (opcional, con un botón en Configuración).
    - Composer 2.10.2 copiado de Laragon a `bin/composer/composer.phar`.
    - Kiltro genera `bin/shims/php.bat` y `composer.bat` que apuntan a la versión de PHP
      elegida; la carpeta `shims` se agrega UNA vez al PATH del usuario (HKCU, sin admin).
      Al cambiar de versión solo se reescriben los .bat (el PATH no se vuelve a tocar).
    - Botón para quitarla del PATH. Avisar que las terminales ya abiertas no lo ven.
- [x] **Etapa 10:** Instalador (NSIS, el que soporta `wails build -nsis`) y versión publicable.
  - Decidido: binarios DENTRO del instalador (funciona sin internet). Solo PHP 8.4 NTS (las demás
    versiones, con el descargador de la Etapa 12). Sin regla de firewall (Nginx escucha en
    127.0.0.1; con "red local" Windows pregunta una vez). Licencia GPL-3.0, copyright Cashupin.
    El repo tiene la receta; el instalador y el zip se suben a GitHub Releases (no al repo).
  - Instalar en una carpeta escribible sin admin (ej. `C:\kiltro`, como Laragon), NO en
    `Program Files`: Kiltro escribe `etc/`, `data/` y `logs/` junto al exe. La versión portable
    (zip) tiene que seguir funcionando igual.
  - Decidido: el archivo hosts se sigue escribiendo con UAC cada vez que cambian los dominios
    (no dar permisos de escritura permanentes sobre el hosts ni correr Kiltro como admin).
  - [x] 10a: Binarios reproducibles y licencias. `tools/bins.json` (versión, URL, SHA-256 de cada
        binario) + `tools/fetch-bins.ps1` (descarga, verifica, descomprime en `bin/`, recorta MySQL;
        mkcert con `go install`). `LICENSE` (GPL-3.0), `THIRD-PARTY-NOTICES.md`, README "Cómo compilar".
  - [x] 10b: Cambios en Kiltro para el instalador: no reinstalar la CA si ya está en el almacén de
        la máquina; flags `--install-ca` (crea la CA y la registra en la máquina) y `--uninstall`
        (quita CA, bloque del hosts y `bin\shims` del PATH).
  - [x] 10c: Script NSIS (`build/windows/installer/project.nsi`): `C:\kiltro`, accesos directos,
        "Agregar o quitar programas", pedir cerrar Kiltro si está abierto, conservar `www\`,
        `data\` y `config.json` al actualizar; desinstalador con casilla "borrar también mis
        proyectos y bases de datos" (desmarcada por defecto).
  - [x] 10d: `tools/release.ps1` (genera `kiltro-setup-<versión>.exe` y `kiltro-portable-<versión>.zip`),
        prueba de instalar/desinstalar en Windows Sandbox, pasos para GitHub Releases.
- [ ] **Etapa 11:** Multi idioma (español e inglés) y publicar la 0.1.0.
      Decidido (2026-11-07): primero los idiomas, después hacer público el repo y publicar la
      0.1.0; el descargador de PHP va en la 0.2.0 (Etapa 12), ya bilingüe desde el principio.
  - Un solo catálogo por idioma (`internal/i18n/locales/es.json` y `en.json`, embebidos en el
    exe) que usan Go (`i18n.T`) y Svelte (`$t`). Sin librerías nuevas. Idioma: automático según
    Windows (español si Windows está en español; si no, inglés) o elegido en Configuración
    (`"lang"` en config.json), cambia sin reiniciar. Los logs quedan en el idioma elegido.
  - [x] 11a: Base (catálogos, `T`/`$t`, detección, selector en Configuración) + textos de las
        3 pantallas y de la bandeja. Test: mismas claves en ambos idiomas y claves usadas existen.
  - [x] 11b: Mensajes del backend (errores, estados, detalles que llegan a la UI).
  - [x] 11c: Instalador bilingüe, `LEEME.txt` en dos idiomas, capturas en inglés para README.en.md.
  - [ ] 11d: Publicar la 0.1.0 (release.ps1, repo público, tag y GitHub Release).
- [ ] **Etapa 12:** Descargador de versiones de PHP (lista de windows.php.net, incluidas las del
      archivo histórico como 7.4; descarga, verifica SHA-256 y deja la carpeta en `bin/php/`).

## Forma de trabajar

- Antes de cambios grandes o de estructura, explicar el plan y esperar confirmación.
- Después de cada cambio, compilar (`go build ./...` y/o `wails build`) y corregir errores antes de terminar.
- Código simple y legible por sobre código "elegante". El dueño viene de PHP/Laravel y tiene
  que poder leer y depurar el Go.
- Comentar las partes no obvias, sobre todo manejo de procesos, Job Objects y elevación.
- Al terminar una tarea: resumen corto de lo hecho, cómo probarlo, y un mensaje de commit sugerido.
- Al completar una etapa, marcarla con [x] en este archivo y actualizar "Estado actual".

## Estado actual

**Etapas 1 a 10 completas, 11a, 11b y 11c** (2026-11-07). Siguiente: 11d (publicar la 0.1.0).

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
- Bandeja (`internal/tray`): `systray.Run` corre en una goroutine con `runtime.LockOSThread()`
  (Windows entrega los mensajes de una ventana solo al hilo que la creó). `HideWindowOnClose`:
  la X esconde; "Salir" llama `runtime.Quit` → `shutdown` (apaga servicios y quita el ícono).
  `SingleInstanceLock` evita dos Kiltro a la vez. El menú se actualiza con el mismo `Notify` de
  los servicios, que llama a `List()`: **nunca llamar `Notify()` con el mutex del servicio tomado**.
  Ícono: `build/windows/icon.ico` (propio desde la 9c).
- HTTPS (`internal/certs`): mkcert 1.4.4 compilado con `go install`. `CAROOT=etc/ssl/ca`,
  `TRUST_STORES=system`; certificados por dominio en `etc/ssl/certs/` (más `localhost` para el
  default server del 443). `"ssl": true` en `config.json` activa `listen 443 ssl` y agrega el 443 a
  `Nginx.Ports()`. Activar/desactivar reinicia Nginx (no `-s reload`: cambia un puerto).
  `mkcert -install` usa `procutil.StartAllowDialogs` (sin HideWindow) para que se vea el aviso de
  Windows. curl de Windows necesita `--ssl-no-revoke` con estos certificados; los navegadores no.
- Configuración (`internal/config/config.go`): `Load()` completa los valores por defecto;
  los servicios usan `config.Current()` (nunca falla). Mientras un servicio corre, `Ports()`
  devuelve los puertos con los que ARRANCÓ (no los de config.json). `SaveSettings` detiene los
  afectados con la config vieja, guarda y los vuelve a iniciar: por eso MySQL se apaga en el
  puerto correcto (y `mysqladmin` recibe `-P`, que pisa el puerto guardado en kiltro-admin.cnf).
- `procutil.CheckPortFree` solo prueba `127.0.0.1`: abrir `0.0.0.0` dispara el aviso del firewall.
- Logs (`internal/logview`): lista FIJA de logs por id (la UI nunca manda rutas). Lee solo los
  últimos 256 KB. Vaciar = truncar a 0; es seguro con el servicio corriendo porque Kiltro, Nginx
  y MySQL escriben en modo append (probado: sin bytes nulos). "Abrir carpeta" usa
  `windows.ShellExecute` para que el Explorador no quede dentro del Job.
- Redis probado con 5.0.14.1 (port de tporadowski, copiado de Laragon) en
  `bin/redis/redis-x64-5.0.14.1/`. `etc/redis/redis.conf` generado en cada arranque (solo
  `127.0.0.1`, datos en `data/redis/dump.rdb`, log en `logs/redis/redis.log`). Apagado con
  `redis-cli -p <puerto con que arrancó> shutdown` (guarda el RDB); matar solo tras 30 s.
  Ninguno de los PHP trae `php_redis.dll`: para Laravel usar `predis` (o agregar la extensión).
- Sitios (9a): la UI manda solo el NOMBRE del sitio; la carpeta sale de `allSites()`. Carpeta y
  Terminal se abren con `procutil.ShellOpen` (fuera del Job, para que no se cierren con Kiltro).
  "Nuevo sitio" valida con `vhosts.CheckNewName` (mismas reglas que `Scan`) y crea la carpeta
  vacía (para que `composer create-project` funcione ahí). "Gestor BD" (`App.OpenAdminer`)
  inicia PHP/MySQL/Nginx y hace `SyncHosts` antes de abrir el navegador.
- UI (9b): colores y estilos comunes en `frontend/src/style.css` (variables en `:root`; botones
  `.primary`, `.outline`, `.ghost`, `.danger`, `.small`; `.card`, `.dot`). Los componentes NO
  deben redefinir `button`/`input` en su `<style>` (los estilos con scope de Svelte le ganan a los
  globales). `BackgroundColour` de `main.go` = `--bg`. Fuente: Segoe UI (la de Windows).
- Ícono (9c): fuente `build/kiltro.svg` → `build/appicon.png`, `build/windows/icon.ico` (9 tamaños,
  armado con `go run ./tools/icopack`) y `frontend/src/assets/images/kiltro.svg` (logo de la UI).
  NO borrar `icon.ico`: `main.go` lo embebe y sin él no compila (wails no alcanza a regenerarlo).
  Pasos para cambiarlo en `build/README.md`.
- php/composer en la terminal (9d): `internal/services/shims.go` genera `bin/shims/php.bat`,
  `composer.bat` y `conf.d/kiltro.ini` (rutas absolutas; se reescriben al iniciar y al cambiar de
  versión). El PHP de consola sin `extension_dir` busca en `C:\php\ext`: los .bat definen
  `PHP_INI_SCAN_DIR` (variable de entorno, la heredan los PHP que lanza Composer; un `-d` no).
  `internal/userpath`: PATH del usuario en `HKCU\Environment` (respeta REG_EXPAND_SZ, va al
  principio, `WM_SETTINGCHANGE` al terminar) y también actualiza el PATH del propio Kiltro.
  Composer 2.10.2 en `bin/composer/composer.phar`.
- Desarrollo: `devmode_dev.go` / `devmode_prod.go` (etiqueta de build `dev`, la que usa
  `wails dev`). `App.GetDevWarning` muestra una franja en la UI si `wails dev` corre sin
  `KILTRO_ROOT` (la raíz sería `build\bin`). En `wails build` no existe.
- Binarios (10a): `tools/bins.json` + `tools/fetch-bins.ps1` (PowerShell 5.1, descomprime con
  `tar.exe` de Windows: el `tar` de Git Bash no abre zip). Para cambiar una versión, actualizar
  `bins.json` (sha256 de la fuente oficial) y el README. MySQL recortado (sin .pdb, .lib,
  include, mecab, plugin/debug): ~275 MB, probado con initialize + REGEXP. Licencia GPL-3.0
  (Cashupin); licencias de terceros en `THIRD-PARTY-NOTICES.md`.
- Instalador (10b): modos sin ventana en `cli.go` (`--write-hosts`, `--remove-ca <pem>`,
  `--install-ca`, `--uninstall`); resultado en `logs/kiltro.log` + código de salida (el exe es
  GUI: no hay consola). `internal/certs/machine.go`: CA en el almacén "Root" de la MÁQUINA con
  la API de Windows (CertOpenStore; leer no pide admin, escribir sí). `EnableSSL` se salta
  `mkcert -install` si la CA ya está en la máquina; `UninstallCA` la quita con UAC
  (`--remove-ca`) y después, si queda, la del usuario. OJO: el "Root" del usuario también
  muestra los de la máquina (`TrustedByUser` da true en ese caso). mkcert solo crea la CA al
  firmar algo (`CreateCA` firma `localhost`). Elevación común en `procutil.RunElevatedAndWait`.
  Para 10c: los archivos que crea `--install-ca` (como admin) heredan permisos de la carpeta de
  instalación: el instalador debe dar Modificar a Usuarios en `C:\kiltro` (Kiltro escribe ahí).
- Instalador (10c): `build/windows/installer/project.nsi` (español, MUI2). Paquete de binarios
  en `build/payload/bin` (ignorado en git; `fetch-bins.ps1 -Dest build\payload\bin`). Versión en
  `wails.json` → `info.productVersion`. `icacls ... *S-1-5-32-545:(OI)(CI)M` (Usuarios por SID).
  "Kiltro abierto" se detecta abriendo kiltro.exe para escritura (falla si está en ejecución).
  "Abrir Kiltro" al final vía `explorer.exe` para que NO corra como admin. Registro en
  HKLM `...\Uninstall\Kiltro` con `InstallLocation` (actualizar = misma carpeta). Probado con
  instalación/actualización/desinstalación silenciosas en `C:\kiltro-prueba` (2026-11-07).
  Con `uninstall.exe /S _?=<dir>` (modo de prueba de NSIS) el uninstall.exe no se borra: normal.
  `project.nsi` tiene que ser UTF-8 CON BOM (sin BOM NSIS lo lee como ANSI: acentos rotos).
  Opciones del instalador (desmarcadas): escritorio y "iniciar con Windows" (HKCU Run →
  `kiltro.exe --tray`, que usa `StartHidden` de Wails). Ojo: en NSIS `$Desktop`/`$Startup` son
  constantes; las variables se llaman `OptDesktop`/`OptStartup`.
- Release (10d): `tools/release.ps1` → `build/release/<versión>/` (setup, zip portable con
  `LEEME.txt` de `tools/portable-LEEME.txt`, `SHA256SUMS.txt` con LF). Versión: SOLO
  `wails.json` → `info.productVersion`; llega al exe con `-ldflags "-X main.version=..."`
  (`version.go`; sin el flag dice "dev"), se ve en "Acerca de" y en el log al iniciar.
  `release.ps1` saca del paquete las carpetas que ya no están en `bins.json`.
  `tools/sandbox.ps1` abre Windows Sandbox con la versión (hay que activar la característica).
  Pasos de publicación en `docs/RELEASING.md`. Los .ps1 van en UTF-8 CON BOM (PowerShell 5.1).
- Idiomas (11a): catálogos `internal/i18n/locales/{es,en}.json` (claves planas tipo
  `"sites.created"`, marcadores `{domain}`). Go: `i18n.T("clave", "domain", d)`. Svelte:
  `import { t, th, tr } from './i18n'` → `{$t('clave', {domain})}`; los textos con `<code>` van
  con `{@html $th(...)}` (escapa los valores); `tr()` en funciones del script. Los textos los
  manda Go (`App.GetI18n`) y `main.ts` los carga ANTES de montar la app. `App.SetLanguage`
  guarda `lang` en config.json, emite `i18n:changed` y rehace la bandeja. Texto nuevo = clave
  en es.json Y en.json; `go test ./internal/i18n` falla si falta una clave, si los marcadores
  no coinciden o si el código usa una clave inexistente. Claves armadas en tiempo de ejecución
  terminan en punto: `$t('status.' + s.status)`, `$t('logs.name.' + l.id)`.
- Mensajes del backend (11b): errores para la UI con `i18n.Err("clave", ...)` o
  `i18n.Wrap(err, "clave", ...)` (el valor usa `{err}`; conserva el error original para
  `errors.Is`). Claves genéricas `err.write`, `err.create`, `err.read`, `err.open`,
  `err.notFound`, `err.stopFailed`, `err.notListening(Log)`. Los errores "centinela"
  (`procutil.ErrCancelled`, `hosts.ErrCancelled`) son tipos con `Error()` que llama a `T`, para
  que salgan en el idioma actual aunque se cambie después. Las líneas del log
  (`Log.Printf("[x] ...")`) siguen en español a propósito; los errores que se loguean salen en
  el idioma en uso. El proceso elevado (`--write-hosts`) no lee config.json: usa el idioma de
  Windows.
