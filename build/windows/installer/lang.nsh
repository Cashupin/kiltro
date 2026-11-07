## Textos del instalador en inglés y español. Lo incluye project.nsi DESPUÉS de
## los MUI_LANGUAGE (LANG_ENGLISH y LANG_SPANISH tienen que existir ya).
## IMPORTANTE: este archivo va en UTF-8 CON BOM, igual que project.nsi.
##
## Para agregar un idioma: un MUI_LANGUAGE más en project.nsi y una línea por
## texto aquí con ${LANG_<IDIOMA>}.


LangString txtLicenseTop ${LANG_ENGLISH} "Kiltro is free software (GPL-3.0). You don't have to accept anything to use it; this license says what you can do when you modify or redistribute it."
LangString txtLicenseTop ${LANG_SPANISH} "Kiltro es software libre (GPL-3.0). No hace falta aceptar nada para usarlo; esta licencia dice qué puedes hacer al modificarlo o redistribuirlo."
LangString txtNext ${LANG_ENGLISH} "Next >"
LangString txtNext ${LANG_SPANISH} "Siguiente >"
LangString txtRunKiltro ${LANG_ENGLISH} "Open Kiltro"
LangString txtRunKiltro ${LANG_SPANISH} "Abrir Kiltro"

LangString txtKiltroOpen ${LANG_ENGLISH} "Kiltro is running.$\r$\n$\r$\nClose it with $\"Quit$\" on its tray icon (next to the clock) and click Retry."
LangString txtKiltroOpen ${LANG_SPANISH} "Kiltro está abierto.$\r$\n$\r$\nCiérralo con $\"Salir$\" en el ícono de la bandeja (junto al reloj) y pulsa Reintentar."
LangString txtNoProgramFiles ${LANG_ENGLISH} "Kiltro can't go in Program Files: it needs to write to its own folder without being administrator.$\r$\n$\r$\nUse a folder like C:\kiltro."
LangString txtNoProgramFiles ${LANG_SPANISH} "Kiltro no puede ir en Archivos de programa: necesita escribir en su carpeta sin ser administrador.$\r$\n$\r$\nUsa una carpeta como C:\kiltro."

LangString txtOptionsTitle ${LANG_ENGLISH} "Options"
LangString txtOptionsTitle ${LANG_SPANISH} "Opciones"
LangString txtOptionsSubtitle ${LANG_ENGLISH} "Choose what else you want (you can change it later)."
LangString txtOptionsSubtitle ${LANG_SPANISH} "Elige qué más quieres (puedes cambiarlo después)."
LangString txtOptDesktop ${LANG_ENGLISH} "Create a desktop shortcut"
LangString txtOptDesktop ${LANG_SPANISH} "Crear un acceso directo en el escritorio"
LangString txtOptStartup ${LANG_ENGLISH} "Start Kiltro with Windows (it stays in the tray, next to the clock)"
LangString txtOptStartup ${LANG_SPANISH} "Iniciar Kiltro con Windows (queda en la bandeja, junto al reloj)"
LangString txtOptStartupHelp ${LANG_ENGLISH} "It only opens Kiltro: services start with their button. Turn it off in Task Manager → Startup apps."
LangString txtOptStartupHelp ${LANG_SPANISH} "Solo abre Kiltro: los servicios se inician con su botón. Se desactiva en el Administrador de tareas → Aplicaciones de arranque."

LangString txtWebView2 ${LANG_ENGLISH} "Installing: WebView2 Runtime"
LangString txtWebView2 ${LANG_SPANISH} "Instalando: WebView2 Runtime"
LangString txtCopying ${LANG_ENGLISH} "Copying Nginx, PHP, MySQL, Redis, Composer, Adminer and mkcert..."
LangString txtCopying ${LANG_SPANISH} "Copiando Nginx, PHP, MySQL, Redis, Composer, Adminer y mkcert..."
LangString txtInstallingCA ${LANG_ENGLISH} "Installing Kiltro's certificate for HTTPS..."
LangString txtInstallingCA ${LANG_SPANISH} "Instalando el certificado de Kiltro para HTTPS..."
LangString txtCAWarning ${LANG_ENGLISH} "Warning: the certificate couldn't be installed (code $0). Details in logs\kiltro.log."
LangString txtCAWarning ${LANG_SPANISH} "Aviso: no se pudo instalar el certificado (código $0). Detalle en logs\kiltro.log."

LangString txtDataTitle ${LANG_ENGLISH} "Your projects and databases"
LangString txtDataTitle ${LANG_SPANISH} "Tus proyectos y bases de datos"
LangString txtDataSubtitle ${LANG_ENGLISH} "Choose whether they are deleted too."
LangString txtDataSubtitle ${LANG_SPANISH} "Elige si también se borran."
LangString txtDataHelp ${LANG_ENGLISH} "By default the www folder (your projects), data (the MySQL and Redis databases) and config.json in $INSTDIR are kept. If you install Kiltro again in the same folder, they'll still be there."
LangString txtDataHelp ${LANG_SPANISH} "Por defecto se conservan la carpeta www (tus proyectos), data (las bases de datos de MySQL y Redis) y config.json en $INSTDIR. Si vuelves a instalar Kiltro en la misma carpeta, siguen ahí."
LangString txtDataCheckbox ${LANG_ENGLISH} "Also delete my projects (www) and databases (data)"
LangString txtDataCheckbox ${LANG_SPANISH} "Borrar también mis proyectos (www) y bases de datos (data)"
LangString txtDataConfirm ${LANG_ENGLISH} "www and data in $INSTDIR will be deleted. This can't be undone.$\r$\n$\r$\nAre you sure?"
LangString txtDataConfirm ${LANG_SPANISH} "Se van a borrar www y data de $INSTDIR. Esto no se puede deshacer.$\r$\n$\r$\n¿Seguro?"
LangString txtCleaning ${LANG_ENGLISH} "Removing the certificate, the hosts file block and the php/composer PATH entry..."
LangString txtCleaning ${LANG_SPANISH} "Quitando el certificado, el bloque del archivo hosts y el PATH de php/composer..."
LangString txtCleanWarning ${LANG_ENGLISH} "Warning: kiltro.exe --uninstall ended with code $0 (details in logs\kiltro.log)."
LangString txtCleanWarning ${LANG_SPANISH} "Aviso: kiltro.exe --uninstall terminó con código $0 (detalle en logs\kiltro.log)."
