<#
.SYNOPSIS
  Arma una versión de Kiltro lista para subir a GitHub Releases.

.DESCRIPTION
  Deja en build\release\<versión>\:
    - kiltro-setup-<versión>.exe     instalador (NSIS)
    - kiltro-portable-<versión>.zip  versión portable (descomprimir y usar)
    - SHA256SUMS.txt                 checksums de los dos, para verificar la descarga

  La versión sale de info.productVersion en wails.json (es la única fuente: el
  instalador, el exe y la pantalla "Acerca de" la toman de ahí).

  Pasos:
    1. tools\fetch-bins.ps1 -Dest build\payload\bin (salta lo que ya está) y borra
       del paquete las carpetas que ya no están en bins.json (ej. una versión vieja).
    2. wails build -nsis con la versión en el exe (-X main.version=...).
    3. Arma el zip portable y los checksums.

  Necesita Go, Node.js, Wails CLI y NSIS. Si makensis no está en el PATH pero
  NSIS está instalado en su carpeta de siempre, se agrega solo para este script.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File tools\release.ps1
#>
param(
    [switch]$SkipFetch  # no descargar/verificar binarios (usar build\payload\bin tal cual)
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$repo = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $repo

$version = (Get-Content wails.json -Raw -Encoding UTF8 | ConvertFrom-Json).info.productVersion
if (-not $version) { throw "wails.json no tiene info.productVersion" }
Write-Host "== Kiltro $version =="

# --- Herramientas ---
$nsisDir = "${env:ProgramFiles(x86)}\NSIS"
if (-not (Get-Command makensis -ErrorAction SilentlyContinue) -and (Test-Path "$nsisDir\makensis.exe")) {
    $env:Path = "$nsisDir;$env:Path"
}
foreach ($tool in "go", "npm", "wails", "makensis") {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        throw "Falta '$tool' en el PATH (ver README, 'Cómo compilar Kiltro desde cero' e 'Instalador')."
    }
}

# Una versión publicada debería salir de un commit, no de cambios sin guardar.
$dirty = & git status --porcelain
if ($dirty) {
    Write-Warning "Hay cambios sin commitear: la versión no va a corresponder exactamente a un commit."
}

# --- 1. Binarios ---
$payload = Join-Path $repo "build\payload\bin"
if (-not $SkipFetch) {
    & (Join-Path $PSScriptRoot "fetch-bins.ps1") -Dest $payload
}
if (-not (Test-Path $payload)) { throw "No existe $payload (corre sin -SkipFetch)" }

# Quitar del paquete lo que ya no está en bins.json (ej. nginx-1.30.4 después de
# subir a otra versión): si no, iría dentro del instalador sin que nadie lo note.
$targets = (Get-Content (Join-Path $PSScriptRoot "bins.json") -Raw -Encoding UTF8 | ConvertFrom-Json).bins |
    ForEach-Object { $_.target }
foreach ($svc in Get-ChildItem $payload -Directory) {
    if ($targets -contains $svc.Name) { continue }  # el target es la carpeta entera (ej. mkcert)
    foreach ($item in Get-ChildItem $svc.FullName) {
        if ($targets -notcontains "$($svc.Name)/$($item.Name)") {
            Write-Host "  quitando del paquete (no está en bins.json): $($svc.Name)\$($item.Name)"
            Remove-Item $item.FullName -Recurse -Force
        }
    }
}

# --- 2. Compilar exe + instalador ---
Write-Host "== wails build -nsis =="
& wails build -nsis -ldflags "-X main.version=$version"
if ($LASTEXITCODE -ne 0) { throw "wails build falló" }

$setup = Join-Path $repo "build\bin\kiltro-setup-$version.exe"
if (-not (Test-Path $setup)) { throw "No se generó $setup" }

# --- 3. Carpeta de la versión ---
$out = Join-Path $repo "build\release\$version"
if (Test-Path $out) { Remove-Item $out -Recurse -Force }
New-Item -ItemType Directory $out | Out-Null
Copy-Item $setup $out

Write-Host "== Zip portable =="
# Se arma en build\release\stage\kiltro\ y se comprime esa carpeta: al
# descomprimir queda una carpeta "kiltro" con todo adentro.
$stageRoot = Join-Path $repo "build\release\stage"
if (Test-Path $stageRoot) { Remove-Item $stageRoot -Recurse -Force }
$stage = Join-Path $stageRoot "kiltro"
New-Item -ItemType Directory $stage, "$stage\www" | Out-Null
Copy-Item (Join-Path $repo "build\bin\kiltro.exe") $stage
Copy-Item $payload "$stage\bin" -Recurse
Copy-Item LICENSE, THIRD-PARTY-NOTICES.md $stage
Copy-Item (Join-Path $PSScriptRoot "portable-LEEME.txt") "$stage\LEEME.txt"

$zip = Join-Path $out "kiltro-portable-$version.zip"
# tar.exe de Windows: -a elige el formato por la extensión (.zip).
& tar.exe -a -c -f $zip -C $stageRoot kiltro
if ($LASTEXITCODE -ne 0) { throw "No se pudo crear $zip" }
Remove-Item $stageRoot -Recurse -Force

# --- 4. Checksums (formato de sha256sum: "<hash>  <archivo>") ---
$sums = Get-ChildItem $out -File | Where-Object Name -ne "SHA256SUMS.txt" | ForEach-Object {
    "{0}  {1}" -f (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower(), $_.Name
}
# Con saltos de línea LF: así `sha256sum -c SHA256SUMS.txt` funciona también en Linux, macOS y Git Bash.
[IO.File]::WriteAllText((Join-Path $out "SHA256SUMS.txt"), (($sums -join "`n") + "`n"))

Write-Host ""
Write-Host "Listo: $out"
Get-ChildItem $out -File | ForEach-Object { "  {0,-34} {1,8:N1} MB" -f $_.Name, ($_.Length / 1MB) }
