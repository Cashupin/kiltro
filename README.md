# Kiltro

Entorno de desarrollo local liviano para Windows (reemplazo de Laragon): Nginx, PHP,
MySQL y Redis portables, con vhosts `*.test` automáticos. Hecho con Wails v2 (Go + Svelte).

## Requisitos para desarrollar

- Go 1.22+
- Node.js + npm
- Wails CLI v2: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- WebView2 (viene con Windows 11)

Verificar con `wails doctor`.

## Binarios (se descargan a mano, no van al repo)

Los binarios van dentro de `bin/` relativo al ejecutable. En desarrollo, eso es
`runtime/bin/` (ver más abajo).

| Servicio | Versión probada | Carpeta                           | Descarga                                   |
|----------|-----------------|-----------------------------------|--------------------------------------------|
| Nginx    | 1.30.4          | `bin/nginx/nginx-1.30.4/nginx.exe` | https://nginx.org/en/download.html (zip de Windows) |
| PHP      | 8.4.27 NTS (x64), 8.3.33 TS (x64) | `bin/php/php-8.4.27-1-nts-Win32-vs17-x64/php-cgi.exe` | https://windows.php.net/download/ (zip; se recomienda **NTS**) |
| MySQL    | 8.4.3 LTS (x64) | `bin/mysql/mysql-8.4.3-winx64/bin/mysqld.exe` | https://dev.mysql.com/downloads/mysql/ (ZIP Archive) |

Se descomprimen los zip tal cual.

- **Nginx:** la carpeta `nginx-<versión>` debe contener `nginx.exe`, `conf/mime.types` y `conf/fastcgi.conf`.
- **PHP:** la carpeta `php-<versión>` debe contener `php-cgi.exe`, `ext/` y `php.ini-development`.
  Si no hay `php.ini`, Kiltro lo crea desde `php.ini-development` con las extensiones típicas
  de Laravel activadas (curl, fileinfo, gd, intl, mbstring, openssl, pdo_mysql, pdo_sqlite,
  sqlite3, zip). Si ya existe, no lo toca. **No copies un `php.ini` de Laragon**: trae rutas de Laragon.
- **MySQL:** la carpeta `mysql-<versión>` debe contener `bin/mysqld.exe` y `bin/mysqladmin.exe`.
  No hace falta `my.ini`: Kiltro genera el suyo en `etc/mysql/my.ini`. Se recomienda el zip
  oficial completo; la copia que trae Laragon no incluye `lib/private` (datos ICU) y MySQL
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

### Gestor de base de datos (Adminer)

| Herramienta | Versión probada | Archivo | Descarga |
|-------------|-----------------|---------|----------|
| Adminer     | 6.3.0 (MySQL)   | `bin/adminer/adminer-6.3.0-mysql.php` | https://www.adminer.org/ (versión "for MySQL") |

- Si el archivo está en `bin/adminer/`, Kiltro sirve Adminer en `http://adminer.test`
  (botón **Gestor BD** en la fila de MySQL; necesita PHP, MySQL y Nginx corriendo).
- Permite entrar como `root` sin contraseña. Por eso `adminer.test` **solo responde desde
  esta PC** (127.0.0.1): nadie de la red local puede entrar.
- El nombre `adminer` queda reservado: una carpeta `www/adminer` se ignora.
- Kiltro le pone su propio tema (claro/oscuro según Windows). El original está en
  `templates/adminer/` y se copia a `etc/adminer/` al recargar sitios.

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

## Sitios y archivo hosts

Cada carpeta dentro de `www/` se sirve en `http://<carpeta>.test` (los proyectos Laravel
se sirven desde `public/`). Los vhosts se generan en `etc/nginx/sites/` al arrancar Nginx
o al pulsar "Recargar sitios".

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

## Tests

```bat
go test ./internal/...
```
