<#
.SYNOPSIS
  Descarga los binarios que usa Kiltro (Nginx, PHP, MySQL, Redis, Composer,
  Adminer, mkcert) y los deja en una carpeta bin\ lista para usar.

.DESCRIPTION
  Qué se descarga está en tools\bins.json: versión, URL y SHA-256 de cada
  archivo. Si el SHA-256 de lo descargado no coincide, el script se detiene
  (archivo corrupto o cambiado en el servidor): nunca se instala algo que no
  sea exactamente lo esperado.

  Lo que ya está en la carpeta destino se salta (usar -Force para rehacerlo).
  mkcert no publica .exe para Windows con checksums, así que se compila desde
  su código con `go install` (Go verifica el módulo con sum.golang.org).

.EXAMPLE
  # Desarrollo: deja todo en runtime\bin (la carpeta de KILTRO_ROOT)
  .\tools\fetch-bins.ps1

.EXAMPLE
  # Otra carpeta (ej. la que se empaqueta en el instalador)
  .\tools\fetch-bins.ps1 -Dest C:\temp\kiltro\bin -Force
#>
param(
    [string]$Dest = (Join-Path $PSScriptRoot "..\runtime\bin"),
    [switch]$Force
)

$ErrorActionPreference = "Stop"
# Invoke-WebRequest muestra una barra de progreso que lo hace MUY lento en PowerShell 5.1.
$ProgressPreference = "SilentlyContinue"

$manifest = Get-Content (Join-Path $PSScriptRoot "bins.json") -Raw -Encoding UTF8 | ConvertFrom-Json
New-Item -ItemType Directory -Force $Dest | Out-Null
$Dest = (Resolve-Path $Dest).Path

# Carpeta temporal propia para las descargas (se borra al final).
$work = Join-Path $env:TEMP ("kiltro-fetch-" + [guid]::NewGuid().ToString("N").Substring(0, 8))
New-Item -ItemType Directory $work | Out-Null

function Get-Download($item) {
    $file = Join-Path $work (Split-Path $item.urls[0] -Leaf)
    foreach ($url in $item.urls) {
        try {
            Write-Host "    descargando $url"
            Invoke-WebRequest -Uri $url -OutFile $file -UseBasicParsing
            break
        } catch {
            # Ej. PHP mueve las versiones viejas a releases/archives: se prueba la siguiente URL.
            Write-Host "    no se pudo ($($_.Exception.Message)), probando la siguiente URL"
            Remove-Item $file -ErrorAction SilentlyContinue
        }
    }
    if (-not (Test-Path $file)) { throw "$($item.name): no se pudo descargar de ninguna URL" }

    $hash = (Get-FileHash $file -Algorithm SHA256).Hash.ToLower()
    if ($hash -ne $item.sha256.ToLower()) {
        throw "$($item.name): el SHA-256 no coincide.`n  esperado: $($item.sha256)`n  obtenido: $hash"
    }
    Write-Host "    SHA-256 OK"
    return $file
}

try {
    foreach ($item in $manifest.bins) {
        $target = Join-Path $Dest $item.target
        Write-Host "* $($item.name) $($item.version)"
        if ((Test-Path $target) -and -not $Force) {
            Write-Host "    ya existe $target (se salta; -Force para rehacer)"
            continue
        }
        if (Test-Path $target) { Remove-Item $target -Recurse -Force }
        New-Item -ItemType Directory -Force (Split-Path $target -Parent) | Out-Null

        switch ($item.kind) {
            "zip" {
                $zip = Get-Download $item
                $out = Join-Path $work ($item.name + "-zip")
                New-Item -ItemType Directory $out | Out-Null
                # tar.exe viene con Windows 10/11 y descomprime zip mucho más rápido que Expand-Archive.
                & tar.exe -xf $zip -C $out
                if ($LASTEXITCODE -ne 0) { throw "$($item.name): no se pudo descomprimir $zip" }
                # zipDir: la carpeta de adentro del zip que se copia ("" = el zip trae los archivos sueltos).
                $src = if ($item.zipDir) { Join-Path $out $item.zipDir } else { $out }
                Move-Item $src $target
            }
            "file" {
                $file = Get-Download $item
                Move-Item $file $target
            }
            "go" {
                # GOBIN = dónde deja go install el .exe.
                Write-Host "    go install $($item.module)"
                New-Item -ItemType Directory -Force $target | Out-Null
                $env:GOBIN = $target
                & go install $item.module
                if ($LASTEXITCODE -ne 0) { throw "$($item.name): go install falló" }
                Remove-Item Env:GOBIN
            }
            default { throw "$($item.name): tipo desconocido '$($item.kind)'" }
        }

        # Archivos que no hacen falta (ej. símbolos de depuración y tests de MySQL).
        foreach ($rel in $item.remove) {
            # Get-Item (no Get-ChildItem): con una carpeta devuelve la carpeta misma, no su contenido.
            Get-Item (Join-Path $target $rel) -ErrorAction SilentlyContinue |
                Remove-Item -Recurse -Force
        }
        Write-Host "    listo: $target"
    }
} finally {
    Remove-Item $work -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host ""
Write-Host "Binarios listos en $Dest"
