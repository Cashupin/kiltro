<#
.SYNOPSIS
  Abre Windows Sandbox con la versión armada por release.ps1, para probar el
  instalador y el zip portable en un Windows limpio (se borra todo al cerrarlo).

.DESCRIPTION
  Genera build\release\kiltro-sandbox.wsb y lo abre. Dentro del Sandbox, la
  carpeta build\release\<versión> aparece en el escritorio como "release"
  (solo lectura: no se puede romper nada del lado de tu PC).

  Windows Sandbox viene con Windows 10/11 Pro, pero hay que activarlo una vez:
  "Activar o desactivar las características de Windows" → "Espacio aislado de
  Windows" (Windows Sandbox) → reiniciar.

  Qué probar dentro:
    1. Instalar con kiltro-setup-<versión>.exe (pantallas, opciones, acentos).
    2. Abrir Kiltro, "Iniciar todo", crear un sitio, activar HTTPS (no debe
       aparecer el aviso del certificado) y abrir https://<sitio>.test.
    3. Desinstalar desde "Agregar o quitar programas".
    4. Descomprimir el zip portable en C:\kiltro y repetir el paso 2.
  El Sandbox no trae el runtime de WebView2 en todas las versiones: si falta,
  el instalador lo descarga (hace falta internet dentro del Sandbox, que viene
  activado por defecto).
#>
param(
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"
$repo = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if (-not $Version) {
    $Version = (Get-Content (Join-Path $repo "wails.json") -Raw -Encoding UTF8 | ConvertFrom-Json).info.productVersion
}
$release = Join-Path $repo "build\release\$Version"
if (-not (Test-Path $release)) { throw "No existe ${release}: corre primero tools\release.ps1" }

$sandboxExe = Join-Path $env:WINDIR "System32\WindowsSandbox.exe"
if (-not (Test-Path $sandboxExe)) {
    throw "Windows Sandbox no está activado. Actívalo en 'Activar o desactivar las características de Windows' → 'Espacio aislado de Windows' y reinicia."
}

$wsb = Join-Path $repo "build\release\kiltro-sandbox.wsb"
@"
<Configuration>
  <MappedFolders>
    <MappedFolder>
      <HostFolder>$release</HostFolder>
      <SandboxFolder>C:\Users\WDAGUtilityAccount\Desktop\release</SandboxFolder>
      <ReadOnly>true</ReadOnly>
    </MappedFolder>
  </MappedFolders>
  <LogonCommand>
    <Command>explorer.exe C:\Users\WDAGUtilityAccount\Desktop\release</Command>
  </LogonCommand>
</Configuration>
"@ | Set-Content $wsb -Encoding UTF8

Write-Host "Abriendo Windows Sandbox con $release ..."
Start-Process $wsb
