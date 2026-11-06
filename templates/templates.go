// Package templates embebe las plantillas dentro del .exe, para que la app
// siga siendo un solo archivo portable.
package templates

import "embed"

// FS contiene las plantillas. Ej: FS.ReadFile("nginx/vhost.conf.tmpl").
// adminer/*.css es el tema de Kiltro para Adminer.
//
//go:embed nginx/*.tmpl adminer/*.css
var FS embed.FS
