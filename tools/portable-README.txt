Kiltro - portable version
=========================

(En español: LEEME.txt)

Lightweight local development environment for Windows: Nginx, PHP, MySQL and
Redis, with an https://<folder>.test site for every folder inside www\.

How to use it
-------------
1. Put this "kiltro" folder somewhere you can write without being
   administrator, for example C:\kiltro. NOT in "Program Files": Kiltro keeps
   its settings, databases and logs right here.
2. Open kiltro.exe.
3. Click "Start all". Your projects go in www\ (one folder per site).

The first time, Windows may ask for administrator permission for the hosts
file (so .test domains work). When you enable HTTPS, Windows asks you to trust
Kiltro's certificate: accept it. (With the installer,
kiltro-setup-<version>.exe, that prompt doesn't appear.)

Kiltro follows your Windows language (Spanish or English). You can change it
in Settings → Language.

Removing it
-----------
1. In Kiltro: Settings → "Uninstall Kiltro's certificate" (if you enabled
   HTTPS) and "Remove from PATH" (if you added php/composer).
2. Remove the "# KILTRO START" ... "# KILTRO END" block from
   C:\Windows\System32\drivers\etc\hosts (or delete your sites and click
   "Update" on the hosts file card).
3. Close Kiltro ("Quit" on the tray icon) and delete the folder.
   Careful: www\ (your projects) and data\ (your databases) are inside.

License: GPL-3.0 (see LICENSE). The bundled programs have their own licenses:
see THIRD-PARTY-NOTICES.md.
Source code: https://github.com/Cashupin/kiltro
