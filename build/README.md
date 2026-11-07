# Build Directory

The build directory is used to house all the build files and assets for your application. 

The structure is:

* bin - Output directory
* darwin - macOS specific files
* windows - Windows specific files

## Mac

The `darwin` directory holds files specific to Mac builds.
These may be customised and used as part of the build. To return these files to the default state, simply delete them
and
build with `wails build`.

The directory contains the following files:

- `Info.plist` - the main plist file used for Mac builds. It is used when building using `wails build`.
- `Info.dev.plist` - same as the main plist file but used when building using `wails dev`.

## Windows

The `windows` directory contains the manifest and rc files used when building with `wails build`.
These may be customised for your application. To return these files to the default state, simply delete them and
build with `wails build`.

- `icon.ico` - The icon used for the application. This is used when building using `wails build`. If you wish to
  use a different icon, simply replace this file with your own. If it is missing, a new `icon.ico` file
  will be created using the `appicon.png` file in the build directory.
- `installer/*` - The files used to create the Windows installer. These are used when building using `wails build`.
- `info.json` - Application details used for Windows builds. The data here will be used by the Windows installer,
  as well as the application itself (right click the exe -> properties -> details)
- `wails.exe.manifest` - The main application manifest file.
## Ícono de Kiltro

La fuente es `build/kiltro.svg` (un quiltro: una oreja parada, otra caída y una mancha en un
ojo). De ahí salen:

- `build/appicon.png` (1024 px).
- `build/windows/icon.ico` (16, 20, 24, 32, 40, 48, 64, 128 y 256 px): ícono del `.exe`, de la
  ventana y de la bandeja (`main.go` lo embebe). **No borrarlo**: `main.go` lo necesita para
  compilar, así que `wails build` no alcanza a regenerarlo solo.
- `frontend/src/assets/images/kiltro.svg`: logo de la barra superior (copia del SVG).

Para cambiar el ícono:

1. Editar `build/kiltro.svg` y copiarlo a `frontend/src/assets/images/kiltro.svg`.
2. Exportar un PNG por tamaño (`icon-16.png` … `icon-256.png`) y `appicon.png` de 1024 px
   (cualquier editor de imágenes, o dibujando el SVG en un `<canvas>` del navegador).
3. Armar el `.ico` (solo usa la librería estándar de Go):

   ```
   go run ./tools/icopack -o build/windows/icon.ico icon-16.png icon-20.png icon-24.png icon-32.png icon-40.png icon-48.png icon-64.png icon-128.png icon-256.png
   ```
