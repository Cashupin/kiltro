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
| PHP      | 8.3.33 (x64)    | `bin/php/php-8.3.33-Win32-vs16-x64/php-cgi.exe` | https://windows.php.net/download/ (zip; se recomienda **NTS**) |

Se descomprimen los zip tal cual. Si hay varias versiones, por ahora se usa la mayor.

- **Nginx:** la carpeta `nginx-<versión>` debe contener `nginx.exe`, `conf/mime.types` y `conf/fastcgi.conf`.
- **PHP:** la carpeta `php-<versión>` debe contener `php-cgi.exe`, `ext/` y `php.ini-development`.
  Si no hay `php.ini`, Kiltro lo crea desde `php.ini-development` con las extensiones típicas
  de Laravel activadas (curl, fileinfo, gd, intl, mbstring, openssl, pdo_mysql, pdo_sqlite,
  sqlite3, zip). Si ya existe, no lo toca. **No copies un `php.ini` de Laragon**: trae rutas de Laragon.

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

## Build

```bat
wails build
```

Genera `build/bin/kiltro.exe`.

## Tests

```bat
go test ./internal/...
```
