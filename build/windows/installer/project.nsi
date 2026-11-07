## IMPORTANTE: este archivo tiene que quedar en UTF-8 CON BOM. Sin el BOM, NSIS lo lee
## como ANSI y los acentos salen mal en el instalador ("tambiÃ©n").
Unicode true

####
## Instalador de Kiltro (NSIS). Lo arma `wails build -nsis` (ver tools/release.ps1).
##
## wails_tools.nsh lo regenera Wails en cada build (con los datos de "info" de
## wails.json): no editarlo. Este archivo (project.nsi) Wails no lo toca.
##
## Qué hace el instalador (corre como admin, una sola vez):
##   1. Copia kiltro.exe y bin\ (los binarios de build\payload\bin, que arma
##      tools\fetch-bins.ps1) a C:\kiltro. NO a Program Files: Kiltro escribe
##      etc\, data\, logs\ y config.json junto al exe.
##   2. Da permiso de Modificar a "Usuarios" en esa carpeta (Kiltro corre SIN admin).
##   3. kiltro.exe --install-ca: pone la CA de HTTPS en el almacén de la máquina
##      (así al activar HTTPS no aparece el aviso de seguridad de Windows).
##   4. Acceso directo en el menú Inicio (escritorio e "iniciar con Windows"
##      son opcionales, desmarcados por defecto) y entrada en "Agregar o quitar programas".
## Al actualizar (instalar encima) se conservan www\, data\, config.json y los
## php.ini (no vienen en el paquete, así que no se pisan).
##
## El desinstalador corre kiltro.exe --uninstall (quita la CA, el bloque del
## hosts y bin\shims del PATH) y borra los programas. www\, data\ y config.json
## solo se borran si se marca la casilla.
####

!define INFO_PRODUCTNAME "Kiltro"
!define UNINST_KEY_NAME  "Kiltro"
!define REQUEST_EXECUTION_LEVEL "admin"

# Carpeta con los binarios a empaquetar (relativa a este archivo).
!ifndef KILTRO_PAYLOAD
    !define KILTRO_PAYLOAD "..\..\payload\bin"
!endif

!include "wails_tools.nsh"
!include "nsDialogs.nsh"
!include "LogicLib.nsh"
!include "StrFunc.nsh"
${StrStr}

# La versión del exe tiene que tener 4 partes.
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"
VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "Instalador de ${INFO_PRODUCTNAME}"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!include "MUI2.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_ABORTWARNING

# --- Páginas del instalador ---
!insertmacro MUI_PAGE_WELCOME
!define MUI_LICENSEPAGE_TEXT_TOP "Kiltro es software libre (GPL-3.0). No hace falta aceptar nada para usarlo; esta licencia dice qué puedes hacer al modificarlo o redistribuirlo."
!define MUI_LICENSEPAGE_BUTTON "Siguiente >"
!insertmacro MUI_PAGE_LICENSE "..\..\..\LICENSE"
!define MUI_PAGE_CUSTOMFUNCTION_LEAVE CheckInstallDir
!insertmacro MUI_PAGE_DIRECTORY
Page custom OptionsPage OptionsPageLeave
!insertmacro MUI_PAGE_INSTFILES
# "Abrir Kiltro" al terminar: SIN admin (ver RunKiltro).
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "Abrir Kiltro"
!define MUI_FINISHPAGE_RUN_FUNCTION RunKiltro
!insertmacro MUI_PAGE_FINISH

# --- Páginas del desinstalador ---
!insertmacro MUI_UNPAGE_CONFIRM
UninstPage custom un.DataPage un.DataPageLeave
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "Spanish"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\kiltro-setup-${INFO_PRODUCTVERSION}.exe"
# Una carpeta en C:\ donde el usuario puede escribir (no Program Files).
InstallDir "C:\kiltro"
ShowInstDetails show
ShowUninstDetails show

Var DeleteData   ; desinstalador: 1 = borrar también www\, data\ y config.json
Var DataCheckbox
Var OptDesktop      ; instalador: 1 = acceso directo en el escritorio
Var OptStartup      ; instalador: 1 = iniciar Kiltro con Windows (en la bandeja)
Var DesktopCheckbox
Var StartupCheckbox

# "Iniciar con Windows": valor en HKCU\...\Run (del usuario, no de toda la máquina).
# --tray = arranca escondido en la bandeja, sin abrir la ventana.
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"

# Espera a que Kiltro esté cerrado. Mientras corre, Windows no deja abrir
# kiltro.exe para escritura (y hay que reemplazarlo o borrarlo). Los servicios
# (nginx, mysqld...) se cierran solos con Kiltro: están en su Job Object.
!macro CheckKiltroClosed
    ${If} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        retry_closed:
        ClearErrors
        FileOpen $0 "$INSTDIR\${PRODUCT_EXECUTABLE}" a
        ${If} ${Errors}
            MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "Kiltro está abierto.$\r$\n$\r$\nCiérralo con $\"Salir$\" en el ícono de la bandeja (junto al reloj) y pulsa Reintentar." /SD IDCANCEL IDRETRY retry_closed
            Abort
        ${EndIf}
        FileClose $0
    ${EndIf}
!macroend

Function .onInit
    !insertmacro wails.checkArchitecture
    # Si ya está instalado, actualizar en la misma carpeta.
    SetRegView 64
    ReadRegStr $0 HKLM "${UNINST_KEY}" "InstallLocation"
    ${If} $0 != ""
        StrCpy $INSTDIR $0
    ${EndIf}

    # Estado actual de las opciones (para la página "Opciones" y para /S).
    StrCpy $OptDesktop 0
    StrCpy $OptStartup 0
    SetShellVarContext all
    ${If} ${FileExists} "$DESKTOP\${INFO_PRODUCTNAME}.lnk"
        StrCpy $OptDesktop 1
    ${EndIf}
    ReadRegStr $0 HKCU "${RUN_KEY}" "${INFO_PRODUCTNAME}"
    ${If} $0 != ""
        StrCpy $OptStartup 1
    ${EndIf}
FunctionEnd

# No dejar instalar en Program Files: ahí un programa sin admin no puede
# escribir, y Kiltro escribe junto al exe.
Function CheckInstallDir
    ${StrStr} $0 "$INSTDIR\" "$PROGRAMFILES64\"
    ${StrStr} $1 "$INSTDIR\" "$PROGRAMFILES32\"
    ${If} $0 != ""
    ${OrIf} $1 != ""
        MessageBox MB_OK|MB_ICONEXCLAMATION "Kiltro no puede ir en Archivos de programa: necesita escribir en su carpeta sin ser administrador.$\r$\n$\r$\nUsa una carpeta como C:\kiltro."
        Abort
    ${EndIf}
FunctionEnd

# Página "Opciones": las dos casillas vienen desmarcadas en una instalación
# nueva. Al actualizar, muestran lo que ya estaba (para no quitar nada sin querer).
# En la instalación silenciosa (/S) esta página no aparece: quedan como estaban.
Function OptionsPage
    !insertmacro MUI_HEADER_TEXT "Opciones" "Elige qué más quieres (puedes cambiarlo después)."
    nsDialogs::Create 1018
    Pop $0
    ${NSD_CreateCheckbox} 0 0 100% 12u "Crear un acceso directo en el escritorio"
    Pop $DesktopCheckbox
    ${NSD_CreateCheckbox} 0 20u 100% 12u "Iniciar Kiltro con Windows (queda en la bandeja, junto al reloj)"
    Pop $StartupCheckbox
    ${NSD_CreateLabel} 12u 34u 95% 24u "Solo abre Kiltro: los servicios se inician con su botón. Se desactiva en el Administrador de tareas → Aplicaciones de arranque."
    Pop $0

    ${If} $OptDesktop == 1
        ${NSD_Check} $DesktopCheckbox
    ${EndIf}
    ${If} $OptStartup == 1
        ${NSD_Check} $StartupCheckbox
    ${EndIf}
    nsDialogs::Show
FunctionEnd

Function OptionsPageLeave
    ${NSD_GetState} $DesktopCheckbox $0
    ${NSD_GetState} $StartupCheckbox $1
    StrCpy $OptDesktop 0
    StrCpy $OptStartup 0
    ${If} $0 == ${BST_CHECKED}
        StrCpy $OptDesktop 1
    ${EndIf}
    ${If} $1 == ${BST_CHECKED}
        StrCpy $OptStartup 1
    ${EndIf}
FunctionEnd

# El instalador corre como admin; si se lanzara kiltro.exe directo, Kiltro
# también correría como admin. Pidiéndole al Explorador que lo abra, se abre
# como el usuario normal.
Function RunKiltro
    Exec '"$WINDIR\explorer.exe" "$INSTDIR\${PRODUCT_EXECUTABLE}"'
FunctionEnd

Section
    !insertmacro wails.setShellContext
    !insertmacro CheckKiltroClosed

    !insertmacro wails.webview2runtime

    # Permiso de Modificar para "Usuarios" (SID S-1-5-32-545, así funciona en
    # Windows en cualquier idioma). (OI)(CI) = lo heredan archivos y carpetas
    # que se creen adentro, incluidos los que copia este instalador.
    CreateDirectory "$INSTDIR"
    nsExec::ExecToLog 'icacls "$INSTDIR" /grant *S-1-5-32-545:(OI)(CI)M /Q'

    SetOutPath $INSTDIR
    !insertmacro wails.files
    File "..\..\..\LICENSE"
    File "..\..\..\THIRD-PARTY-NOTICES.md"

    DetailPrint "Copiando Nginx, PHP, MySQL, Redis, Composer, Adminer y mkcert..."
    SetOutPath "$INSTDIR\bin"
    File /r "${KILTRO_PAYLOAD}\*.*"
    SetOutPath $INSTDIR

    CreateDirectory "$INSTDIR\www"

    DetailPrint "Instalando el certificado de Kiltro para HTTPS..."
    ExecWait '"$INSTDIR\${PRODUCT_EXECUTABLE}" --install-ca' $0
    ${If} $0 != 0
        # No es grave: al activar HTTPS, Kiltro lo instala para el usuario (con el aviso de Windows).
        DetailPrint "Aviso: no se pudo instalar el certificado (código $0). Detalle en logs\kiltro.log."
    ${EndIf}

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    ${If} $OptDesktop == 1
        CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    ${Else}
        Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"
    ${EndIf}

    # HKCU es el del usuario que aceptó el UAC (normalmente el mismo que usa la PC).
    ${If} $OptStartup == 1
        WriteRegStr HKCU "${RUN_KEY}" "${INFO_PRODUCTNAME}" '"$INSTDIR\${PRODUCT_EXECUTABLE}" --tray'
    ${Else}
        DeleteRegValue HKCU "${RUN_KEY}" "${INFO_PRODUCTNAME}"
    ${EndIf}

    !insertmacro wails.writeUninstaller
    SetRegView 64
    WriteRegStr HKLM "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
    WriteRegStr HKLM "${UNINST_KEY}" "URLInfoAbout" "https://github.com/Cashupin/kiltro"
SectionEnd

# --- Desinstalador ---

Function un.DataPage
    !insertmacro MUI_HEADER_TEXT "Tus proyectos y bases de datos" "Elige si también se borran."
    nsDialogs::Create 1018
    Pop $0
    ${NSD_CreateLabel} 0 0 100% 36u "Por defecto se conservan la carpeta www (tus proyectos), data (las bases de datos de MySQL y Redis) y config.json en $INSTDIR. Si vuelves a instalar Kiltro en la misma carpeta, siguen ahí."
    Pop $0
    ${NSD_CreateCheckbox} 0 44u 100% 12u "Borrar también mis proyectos (www) y bases de datos (data)"
    Pop $DataCheckbox
    nsDialogs::Show
FunctionEnd

Function un.DataPageLeave
    ${NSD_GetState} $DataCheckbox $DeleteData
    ${If} $DeleteData == ${BST_CHECKED}
        MessageBox MB_YESNO|MB_ICONEXCLAMATION "Se van a borrar www y data de $INSTDIR. Esto no se puede deshacer.$\r$\n$\r$\n¿Seguro?" IDYES +2
        Abort
    ${EndIf}
FunctionEnd

Section "uninstall"
    !insertmacro wails.setShellContext
    !insertmacro CheckKiltroClosed

    DetailPrint "Quitando el certificado, el bloque del archivo hosts y el PATH de php/composer..."
    ExecWait '"$INSTDIR\${PRODUCT_EXECUTABLE}" --uninstall' $0
    ${If} $0 != 0
        DetailPrint "Aviso: kiltro.exe --uninstall terminó con código $0 (detalle en logs\kiltro.log)."
    ${EndIf}

    # Los backups del hosts se guardan aparte antes de borrar etc\ (por si acaso).
    ${If} ${FileExists} "$INSTDIR\etc\hosts-backup\*.*"
        Rename "$INSTDIR\etc\hosts-backup" "$INSTDIR\hosts-backup"
    ${EndIf}

    RMDir /r "$INSTDIR\bin"
    RMDir /r "$INSTDIR\etc"
    RMDir /r "$INSTDIR\logs"
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
    Delete "$INSTDIR\LICENSE"
    Delete "$INSTDIR\THIRD-PARTY-NOTICES.md"

    ${If} $DeleteData == ${BST_CHECKED}
        RMDir /r "$INSTDIR\www"
        RMDir /r "$INSTDIR\data"
        RMDir /r "$INSTDIR\hosts-backup"
        Delete "$INSTDIR\config.json"
    ${EndIf}

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"
    DeleteRegValue HKCU "${RUN_KEY}" "${INFO_PRODUCTNAME}"
    !insertmacro wails.deleteUninstaller

    # Solo se borra si quedó vacía (si se conservaron www/data, queda).
    RMDir "$INSTDIR"

    # Datos de WebView2 de la ventana (caché), en el %APPDATA% del usuario.
    SetShellVarContext current
    RMDir /r "$APPDATA\${PRODUCT_EXECUTABLE}"
SectionEnd
