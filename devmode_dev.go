//go:build dev

package main

// Este archivo solo entra en las compilaciones de `wails dev` (que compila con
// la etiqueta "dev"). En `wails build` se usa devmode_prod.go.
const devBuild = true
