// Package templates embebe las plantillas dentro del .exe, para que la app
// siga siendo un solo archivo portable.
package templates

import "embed"

// FS contiene las plantillas. Ej: FS.ReadFile("nginx/vhost.conf.tmpl").
//
//go:embed nginx/*.tmpl
var FS embed.FS
