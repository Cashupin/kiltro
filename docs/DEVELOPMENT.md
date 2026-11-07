# Kiltro — guía técnica

Cómo está armado Kiltro, cómo compilarlo y cómo funciona cada parte por dentro. Para
instalarlo y usarlo, ver el [README](../README.md).

Kiltro es una app de escritorio hecha con [Wails v2](https://wails.io) (Go + Svelte) que
levanta Nginx, PHP (php-cgi), MySQL y Redis portables, sin servicios de Windows ni Docker.

## Cómo compilar Kiltro desde cero

1. Instalar lo de "Requisitos para desarrollar" (Go, Node.js, Wails CLI).
2. Clonar el repo y descargar los binarios: `.\tools\fetch-bins.ps1` (unos 400 MB).
3. Desarrollo: `$env:KILTRO_ROOT = "$PWD\runtime"; wails dev`.
4. Compilar: `wails build` (sale `build\bin\kiltro.exe`).

## Requisitos para desarrollar

- Go 1.22+
- Node.js + npm
- Wails CLI v2: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- WebView2 (viene con Windows 11)

Verificar con `wails doctor`.

## Binarios (no van al repo)

Los binarios van dentro de `bin/` relativo al ejecutable. En desarrollo, eso es
`runtime/bin/` (ver más abajo).

**La forma fácil:** el script descarga todo, verifica el SHA-256 de cada archivo y lo deja
en su carpeta (lo que ya está, se salta):

```powershell
.\tools\fetch-bins.ps1                  # a runtime\bin
.\tools\fetch-bins.ps1 -Dest D:\x\bin   # a otra carpeta
```

Las versiones, URLs y checksums están en [`tools/bins.json`](../tools/bins.json). Necesita
Go en el PATH (mkcert se compila con `go install`). Si PowerShell no deja correr scripts, usar
`powershell -ExecutionPolicy Bypass -File tools\fetch-bins.ps1`. A mano, son estos (Composer, Adminer y
mkcert, en sus secciones más abajo):

| Servicio | Versión probada | Carpeta                           | Descarga                                   |
|----------|-----------------|-----------------------------------|--------------------------------------------|
| Nginx    | 1.30.4          | `bin/nginx/nginx-1.30.4/nginx.exe` | https://nginx.org/en/download.html (zip de Windows) |
| PHP      | 8.4.27 NTS (x64), 8.3.33 TS (x64) | `bin/php/php-8.4.27-1-nts-Win32-vs17-x64/php-cgi.exe` | https://windows.php.net/download/ (zip; se recomienda **NTS**) |
| MySQL    | 8.4.3 LTS (x64) | `bin/mysql/mysql-8.4.3-winx64/bin/mysqld.exe` | https://dev.mysql.com/downloads/mysql/ (ZIP Archive) |
| Redis    | 5.0.14.1 (x64)  | `bin/redis/redis-x64-5.0.14.1/redis-server.exe` | https://github.com/tporadowski/redis/releases (zip; port no oficial para Windows) |

Se descomprimen los zip tal cual.

- **Nginx:** la carpeta `nginx-<versión>` debe contener `nginx.exe`, `conf/mime.types` y `conf/fastcgi.conf`.
- **PHP:** la carpeta `php-<versión>` debe contener `php-cgi.exe`, `ext/` y `php.ini-development`.
  Si no hay `php.ini`, Kiltro lo crea desde `php.ini-development` con las extensiones típicas
  de Laravel activadas (curl, fileinfo, gd, intl, mbstring, openssl, pdo_mysql, pdo_sqlite,
  sqlite3, zip). Si ya existe, no lo toca. **No copies un `php.ini` de otra instalación**: suele traer rutas de esa instalación.
- **MySQL:** la carpeta `mysql-<versión>` debe contener `bin/mysqld.exe` y `bin/mysqladmin.exe`.
  No hace falta `my.ini`: Kiltro genera el suyo en `etc/mysql/my.ini`. Usar el zip oficial
  completo: si falta `lib/private` (datos ICU), MySQL
  muestra una advertencia al arrancar (lo básico, incluido `REGEXP`, funciona igual).

### Varias versiones de PHP

- Cada versión va en su propia carpeta dentro de `bin/php/` (ej. `php-8.3.33-.../` y
  `php-8.4.27-1-nts-.../`). Se elige con el desplegable de la fila de PHP; si PHP está
  corriendo, se reinicia con la nueva versión (Nginx sigue igual).
- La elección se guarda en `config.json`, junto al exe. Sin elección, se usa la versión más nueva.
- Cada carpeta tiene su propio `php.ini` y sus extensiones (`ext/`): un cambio en el
  `php.ini` de 8.3 no afecta a 8.4.
- Se recomienda la build **NTS** (Non Thread Safe): es la indicada para FastCGI y algo más
  liviana. El desplegable muestra (NTS) o (TS) al lado de cada versión.
- Las builds `vs17` necesitan el "Visual C++ Redistributable 2015-2022" (casi siempre ya
  está instalado en Windows 10/11).

### MySQL

- La primera vez, si `data/mysql` está vacía, Kiltro la inicializa: usuario **`root` sin
  contraseña** en `127.0.0.1:3306` (lo que espera el `.env` por defecto de Laravel).
- Kiltro crea además un usuario interno `kiltro_admin` que **solo puede apagar MySQL**; su
  contraseña queda en `etc/mysql/kiltro-admin.cnf`. Así el apagado limpio sigue funcionando
  aunque le pongas contraseña a `root`. No lo borres.
- MySQL se detiene siempre con `mysqladmin shutdown`. Solo si no termina en 60 s se mata el
  proceso (queda una ADVERTENCIA en `logs/kiltro.log`).
- Log de MySQL: `logs/mysql/error.log`.

### Redis

- Redis no publica versiones oficiales para Windows: se usa el port de tporadowski (5.0.14.1),
  que es estable para desarrollo (ya no recibe versiones nuevas).
- Escucha solo en `127.0.0.1:6379` (puerto configurable). Kiltro genera `etc/redis/redis.conf`.
- Los datos se guardan en `data/redis/dump.rdb` cada tanto y siempre al detenerlo
  (`redis-cli shutdown`). Solo si no termina en 30 s se mata el proceso.
- Log: `logs/redis/redis.log` (también en la pestaña Logs).
- Para Laravel: `REDIS_HOST=127.0.0.1`, `REDIS_PORT=6379`. Si usas `REDIS_CLIENT=phpredis`
  necesitas la extensión `php_redis.dll` de tu versión de PHP; con `predis` (paquete de
  Composer) no hace falta nada más.

### Gestor de base de datos (Adminer)

| Herramienta | Versión probada | Archivo | Descarga |
|-------------|-----------------|---------|----------|
| Adminer     | 6.3.0 (MySQL)   | `bin/adminer/adminer-6.3.0-mysql.php` | https://www.adminer.org/ (versión "for MySQL") |

- Si el archivo está en `bin/adminer/`, Kiltro sirve Adminer en `http://adminer.test`
  (botón **Gestor BD** en la fila de MySQL: si falta PHP, MySQL o Nginx, los inicia antes de abrirlo).
- Permite entrar como `root` sin contraseña. Por eso `adminer.test` **solo responde desde
  esta PC** (127.0.0.1): nadie de la red local puede entrar.
- El nombre `adminer` queda reservado: una carpeta `www/adminer` se ignora.
- Kiltro le pone su propio tema (claro/oscuro según Windows). El original está en
  `templates/adminer/` y se copia a `etc/adminer/` al recargar sitios.

### php y composer en la terminal

| Herramienta | Versión probada | Archivo | Descarga |
|-------------|-----------------|---------|----------|
| Composer    | 2.10.2          | `bin/composer/composer.phar` | https://getcomposer.org/download/ ("Latest stable", el `.phar`) |

- Kiltro genera `bin/shims/php.bat` y `bin/shims/composer.bat`, que llaman al `php.exe` de la
  versión elegida en Kiltro. Se reescriben al iniciar Kiltro y al cambiar de versión.
- En **Configuración → php y composer en la terminal**, "Agregar al PATH" pone `bin\shims` al
  principio del PATH de **tu usuario** (registro `HKCU\Environment`, no pide admin). Va
  primero para ganarle a otros PHP instalados (winget, scoop u otros entornos). "Quitar del PATH" lo saca.
- Las terminales que ya estaban abiertas no ven el cambio: hay que cerrarlas y abrir una nueva.
  El botón **Terminal** de un sitio sí lo ve enseguida.
- Usan el mismo `php.ini` de esa versión. El `extension_dir` lo pone Kiltro con
  `PHP_INI_SCAN_DIR` (`bin/shims/conf.d/kiltro.ini`), así que también funciona en los PHP que
  lanza Composer (ej. `@php artisan package:discover`).
- Si llamas `php` o `composer` desde **otro `.bat`**, usa `call php ...` / `call composer ...`
  (sin `call`, tu `.bat` termina ahí: es cómo funciona `cmd` con los `.bat`).
- `composer self-update` actualiza `bin/composer/composer.phar`.

## Desarrollo

La carpeta `runtime/` hace de "carpeta de la app" en desarrollo (bin, etc, www, data, logs).
Se le indica a Kiltro con la variable `KILTRO_ROOT`:

```bat
:: cmd
set KILTRO_ROOT=%CD%\runtime
wails dev
```

```powershell
# PowerShell
$env:KILTRO_ROOT = "$PWD\runtime"
wails dev
```

Sin `KILTRO_ROOT`, Kiltro usa la carpeta donde está `kiltro.exe`.

El log de la app queda en `<raíz>/logs/kiltro.log`.

## Logs

Pestaña **Logs**: elige el archivo (Kiltro, Nginx errores, Nginx accesos, MySQL errores) y
muestra las últimas 500 líneas, actualizándose cada 2 segundos. Las líneas de error se ven en
rojo y las advertencias en amarillo.

- Los **errores de PHP** aparecen en "Nginx: errores" (PHP se los manda a Nginx por FastCGI).
- **Vaciar** deja el archivo en 0 bytes; se puede hacer con el servicio corriendo.
- **Abrir carpeta** abre `logs\` en el Explorador.

## Sitios y archivo hosts

Cada carpeta dentro de `www/` se sirve en `http://<carpeta>.test` (los proyectos Laravel
se sirven desde `public/`). Los vhosts se generan en `etc/nginx/sites/` al arrancar Nginx
o al pulsar "Recargar sitios".

- **Nuevo sitio**: crea la carpeta vacía en `www/` (el nombre se normaliza: "Mi Tienda" → `mi-tienda.test`)
  y recarga los sitios. Pon tu proyecto dentro de esa carpeta.
- Cada sitio tiene botones **Abrir** (navegador), **Carpeta** (Explorador) y **Terminal**
  (Windows Terminal si está instalado; si no, PowerShell) abierta en la carpeta del proyecto.
- Adminer no aparece en la lista: se abre con su botón.

Para que `*.test` resuelva, Kiltro escribe un bloque en
`C:\Windows\System32\drivers\etc\hosts`:

```
# KILTRO START
127.0.0.1 proyecto.test
# KILTRO END
```

- Solo toca lo que está entre esas marcas; el resto del archivo no se modifica.
- Escribir el hosts requiere admin: Windows muestra el aviso de UAC, y solo cuando la
  lista de dominios cambió. Kiltro en sí nunca corre como admin.
- Antes de cada cambio se guarda una copia en `etc/hosts-backup/` (las últimas 10).
- Algunos antivirus avisan cuando un programa modifica el hosts; hay que permitirlo.

## Configuración

Pestaña **Configuración** (se guarda en `config.json`, junto al exe):

| Opción | Por defecto |
|---|---|
| Puerto HTTP / HTTPS de Nginx | 80 / 443 |
| Puerto de MySQL | 3306 |
| Puerto de Redis | 6379 |
| PHP: primer puerto y cantidad de workers | 9000, 4 (puertos 9000-9003) |
| Accesible desde la red local | apagado |
| Carpeta de proyectos | `www\` junto al exe |

- Al guardar se validan los puertos (que no se repitan y que estén libres) y se reinician
  solos los servicios afectados que estaban corriendo.
- **Red local apagada** (recomendado): Nginx escucha solo en `127.0.0.1`; nadie más de tu red
  ve tus proyectos y Windows no pregunta por el firewall. Encendida: escucha en todas las
  interfaces (útil para probar desde el celular con la IP de la PC). Adminer siempre es solo local.
- Si cambias el puerto de MySQL, actualiza `DB_PORT` en el `.env` de tus proyectos.
- Si cambias la carpeta de proyectos, cambian los dominios: Kiltro actualiza el archivo hosts (UAC).
- También ahí: **Desinstalar certificado de Kiltro** (quita la CA de HTTPS de Windows).

## HTTPS (mkcert)

| Herramienta | Versión probada | Archivo | Cómo obtenerlo |
|-------------|-----------------|---------|----------------|
| mkcert      | 1.4.4           | `bin/mkcert/mkcert.exe` | `go install filippo.io/mkcert@v1.4.4` con `GOBIN` apuntando a `bin/mkcert` (Go verifica el código contra sum.golang.org), o el `.exe` de https://github.com/FiloSottile/mkcert/releases |

- Botón **Activar HTTPS** (debajo del archivo hosts). Windows muestra un aviso de seguridad
  preguntando si confías en el certificado "mkcert ..." de Kiltro: hay que aceptar. No
  requiere admin (se instala para tu usuario).
- **Con el instalador** no aparece ese aviso: el instalador (que corre como admin) pone la CA
  en el almacén de la **máquina** y Kiltro, al ver que ya está ahí, se salta `mkcert -install`.
  Al quitarla desde Configuración, Windows pide permiso de administrador (UAC) en vez del aviso.
- Después, todos los sitios responden también por `https://proyecto.test` (y `http://` sigue
  funcionando). Los proyectos nuevos reciben su certificado al pulsar "Recargar sitios".
- Necesita el **puerto 443** libre (VMware o algunos antivirus lo usan).
- La autoridad certificadora (CA) queda en `etc/ssl/ca/` y los certificados en
  `etc/ssl/certs/`. **No compartas `rootCA-key.pem`**: con ese archivo se pueden crear
  certificados que tu PC acepta para cualquier sitio.
- Para quitar la CA de Windows: botón en **Configuración**, o a mano en `certmgr.msc` (la del usuario) o `certlm.msc` (la de la máquina) → Entidades de certificación raíz de confianza →
  Certificados → borrar el que empieza con "mkcert".
- Firefox usa los certificados de Windows por defecto (`security.enterprise_roots.enabled`).
- Para Laravel, recuerda poner `APP_URL=https://proyecto.test` en el `.env` si usas HTTPS.

## Modos sin ventana de kiltro.exe

Los usa el instalador (y la propia app, con UAC). No abren la ventana: hacen la tarea, dejan
el resultado en `logs/kiltro.log` y salen con código 0 si todo salió bien.

| Comando | Quién lo usa | Qué hace |
|---------|--------------|----------|
| `kiltro.exe --install-ca` | Instalador (admin) | Crea la CA de Kiltro y la pone en el almacén de la máquina |
| `kiltro.exe --uninstall` | Desinstalador (admin) | Quita la CA de la máquina, el bloque del hosts y `bin\shims` del PATH |
| `kiltro.exe --write-hosts <archivo>` | Kiltro (UAC) | Escribe el bloque del hosts |
| `kiltro.exe --remove-ca <rootCA.pem>` | Kiltro (UAC) | Quita la CA del almacén de la máquina |
| `kiltro.exe --tray` | "Iniciar con Windows" | Abre Kiltro escondido en la bandeja (este SÍ abre la app) |

## Bandeja del sistema

- Cerrar la ventana con la **X** no cierra Kiltro: la esconde en la bandeja (junto al
  reloj) y los servicios siguen corriendo.
- **Clic** en el ícono: abre la ventana. **Clic derecho**: menú con cada servicio (el tick
  indica que corre; clic para iniciar/detener), "Iniciar todo", "Detener todo" y **Salir**.
- **Salir** detiene todos los servicios de forma limpia y cierra Kiltro.
- Si el ícono no aparece, puede estar en los íconos ocultos (flecha `^` de la barra de tareas).
- Solo puede haber un Kiltro abierto: abrir el `.exe` otra vez muestra la ventana del que ya corre.

## Build

```bat
wails build
```

Genera `build/bin/kiltro.exe`.

## Instalador

```powershell
.\tools\fetch-bins.ps1 -Dest build\payload\bin   # los binarios que van adentro (una vez)
wails build -nsis                                 # sale build\bin\kiltro-setup-<versión>.exe
```

Necesita NSIS (`winget install NSIS.NSIS`) con `C:\Program Files (x86)\NSIS` en el PATH. La
versión sale de `info.productVersion` en `wails.json`. El script es
`build/windows/installer/project.nsi` (`wails_tools.nsh` lo regenera Wails: no editarlo).

Para armar una versión completa (instalador + zip portable + checksums) se usa
`tools\release.ps1`. Los pasos para publicarla en GitHub están en
[`docs/RELEASING.md`](RELEASING.md).

Qué hace el instalador (pide admin una vez):
- Instala en `C:\kiltro` (no se deja elegir Archivos de programa: Kiltro escribe junto al exe)
  y da permiso de Modificar a Usuarios en esa carpeta.
- Copia `kiltro.exe` y `bin\`, crea `www\`, el acceso directo del menú Inicio y la
  entrada en "Agregar o quitar programas".
- Pone la CA de HTTPS en el almacén de la máquina (`kiltro.exe --install-ca`).
- Opciones (desmarcadas por defecto; al actualizar muestran lo que ya estaba): **acceso
  directo en el escritorio** e **iniciar con Windows** (valor `Kiltro` en
  `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` con `kiltro.exe --tray`: arranca
  escondido en la bandeja, sin iniciar servicios). Se desactiva también en el Administrador de
  tareas → Aplicaciones de arranque.
- Si Kiltro está abierto, pide cerrarlo. Instalar encima (actualizar) conserva `www\`, `data\`,
  `config.json` y los `php.ini`.
- "Abrir Kiltro" al terminar lo abre **sin** admin.

El desinstalador quita la CA, el bloque del hosts y `bin\shims` del PATH (`--uninstall`) y
borra los programas. `www\`, `data\` y `config.json` se conservan salvo que se marque
"Borrar también mis proyectos y bases de datos". Los backups del hosts quedan en `hosts-backup\`.

Instalación silenciosa: `kiltro-setup-<versión>.exe /S /D=C:\kiltro` · desinstalar: `C:\kiltro\uninstall.exe /S`.

El instalador está en inglés y español: usa el idioma de Windows (si no es ninguno de los dos,
inglés). Para probar el otro: `kiltro-setup-<versión>.exe /LANG=en` (o `/LANG=es`). Los textos
están en `build/windows/installer/lang.nsh` (UTF-8 con BOM).

## Tests

```bat
go test ./internal/...
```

## Licencia

GPL-3.0, Copyright (C) 2026 Cashupin. Ver [`LICENSE`](../LICENSE) y
[`THIRD-PARTY-NOTICES.md`](../THIRD-PARTY-NOTICES.md).
