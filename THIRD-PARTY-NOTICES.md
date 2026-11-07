# Programas de terceros incluidos en Kiltro

Kiltro se distribuye bajo la licencia GPL-3.0 (ver `LICENSE`). El instalador y el zip
portable incluyen además los siguientes programas, **sin modificar**. Cada uno mantiene
su propia licencia y sus autores; Kiltro solo los lanza como procesos aparte.

La versión exacta de cada uno, de dónde se descarga y su SHA-256 están en
`tools/bins.json`. La única diferencia con el zip original es que se quitan archivos
que no hacen falta para usarlos (símbolos de depuración `.pdb`, librerías para
compilar, tests y diccionarios japoneses de MySQL).

| Programa | Versión | Licencia | Archivo de licencia | Sitio |
|----------|---------|----------|---------------------|-------|
| Nginx | 1.30.4 | BSD-2-Clause (incluye OpenSSL, PCRE y zlib, con sus licencias) | `bin/nginx/nginx-1.30.4/docs/` | https://nginx.org |
| PHP | 8.4.27 NTS | PHP License v3.01 (y las de sus librerías) | `bin/php/php-8.4.27-1-nts-Win32-vs17-x64/license.txt` | https://www.php.net |
| MySQL Community Server | 8.4.3 LTS | GPL-2.0 (con la "Universal FOSS Exception" de Oracle) | `bin/mysql/mysql-8.4.3-winx64/LICENSE` | https://dev.mysql.com |
| Redis para Windows (port de Tomasz Poradowski) | 5.0.14.1 | BSD-3-Clause | https://github.com/tporadowski/redis/blob/win-5.0.14.1/COPYING | https://github.com/tporadowski/redis |
| Composer | 2.10.2 | MIT | https://github.com/composer/composer/blob/main/LICENSE | https://getcomposer.org |
| Adminer | 6.3.0 | Apache-2.0 o GPL-2.0 | https://github.com/vrana/adminer | https://www.adminer.org |
| mkcert | 1.4.4 | BSD-3-Clause | https://github.com/FiloSottile/mkcert/blob/v1.4.4/LICENSE | https://github.com/FiloSottile/mkcert |

El código fuente de MySQL (GPL-2.0) está disponible en https://github.com/mysql/mysql-server
(tag `mysql-8.4.3`) y en https://dev.mysql.com/downloads/mysql/ ("Source Code").

## Librerías compiladas dentro de kiltro.exe

| Librería | Licencia |
|----------|----------|
| Wails v2 (github.com/wailsapp/wails) | MIT |
| fyne.io/systray | Apache-2.0 |
| golang.org/x/sys | BSD-3-Clause |
| Svelte (interfaz) | MIT |
| Go (biblioteca estándar) | BSD-3-Clause |

Las licencias completas de las librerías de Go están en el código de cada módulo
(`go mod download` y luego la carpeta del módulo).
