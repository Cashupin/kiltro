# Cómo publicar una versión de Kiltro

Los instaladores **no van al repo**: se suben como archivos de un *Release* de GitHub
(https://github.com/Cashupin/kiltro/releases). El repo tiene todo lo necesario para
volver a armarlos.

## 1. Preparar

1. Subir la versión en `wails.json` → `info.productVersion` (ej. `0.1.0` → `0.2.0`).
   Es el único lugar: el instalador, el exe y "Acerca de" la toman de ahí.
   Formato `MAYOR.MENOR.PARCHE` ([semver](https://semver.org/lang/es/)):
   - PARCHE (`0.1.1`): solo arreglos.
   - MENOR (`0.2.0`): funciones nuevas.
   - MAYOR (`1.0.0`): cambios que rompen algo (ej. otra estructura de carpetas).
2. Si cambió algún binario, actualizar `tools/bins.json` (y el README).
3. Commitear todo. `release.ps1` avisa si hay cambios sin commitear.

## 2. Armar

```powershell
powershell -ExecutionPolicy Bypass -File tools\release.ps1
```

Deja en `build\release\<versión>\`:

| Archivo | Qué es |
|---------|--------|
| `kiltro-setup-<versión>.exe` | Instalador (recomendado) |
| `kiltro-portable-<versión>.zip` | Versión portable: descomprimir y usar |
| `SHA256SUMS.txt` | Checksums de los dos |

## 3. Probar

Con Windows Sandbox activado (ver `tools\sandbox.ps1`):

```powershell
powershell -ExecutionPolicy Bypass -File tools\sandbox.ps1
```

Abre un Windows limpio con la carpeta de la versión en el escritorio. Probar: instalar,
"Iniciar todo", un sitio por HTTPS (sin aviso del certificado), desinstalar; y el zip
portable en `C:\kiltro`.

## 4. Publicar

1. Crear el tag y subirlo:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```
2. En GitHub: **Releases → Draft a new release** → elegir el tag `v0.1.0` → título
   `Kiltro 0.1.0` → arrastrar los 3 archivos de `build\release\0.1.0\` → pegar las notas
   (plantilla abajo) → **Publish release**.

   Con la CLI de GitHub (`winget install GitHub.cli`), lo mismo en un comando:
   ```bash
   gh release create v0.1.0 build/release/0.1.0/* --title "Kiltro 0.1.0" --notes-file notas.md
   ```

Mientras el repo sea **privado**, los releases también lo son: solo los ve quien tiene
acceso. Al hacerlo público, quedan descargables para todos.

## Plantilla de notas

```markdown
Entorno de desarrollo local liviano para Windows 10/11: Nginx, PHP 8.4, MySQL 8.4 y
Redis, con un sitio `https://<carpeta>.test` por cada carpeta de `www\`.

### Descargas
- **kiltro-setup-0.1.0.exe** — instalador (recomendado). Instala en `C:\kiltro`.
- **kiltro-portable-0.1.0.zip** — portable: descomprimir en una carpeta propia (ej. `C:\kiltro`)
  y abrir `kiltro.exe`.

### Novedades
- ...

### Aviso de Windows al descargar
El instalador todavía no está firmado digitalmente, así que Windows puede mostrar
"Windows protegió tu PC". Pulsa **Más información → Ejecutar de todas formas**.
Puedes verificar el archivo con `SHA256SUMS.txt`:
`Get-FileHash kiltro-setup-0.1.0.exe` (PowerShell) tiene que dar el mismo valor.

Incluye Nginx 1.30.4, PHP 8.4.27 NTS, MySQL 8.4.3, Redis 5.0.14.1 (port de tporadowski),
Composer 2.10.2, Adminer 6.3.0 y mkcert 1.4.4, sin modificar (ver THIRD-PARTY-NOTICES.md).
Kiltro es software libre (GPL-3.0).
```

## Pendiente (más adelante)

- Firmar el instalador (quita el aviso de SmartScreen). Hay opciones gratis para proyectos
  open source, como SignPath Foundation.
- Armar el release automático con GitHub Actions al subir un tag.
