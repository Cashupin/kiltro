//go:build !dev

package main

// Compilación normal (`wails build`, instalador, zip portable): Kiltro usa
// siempre la carpeta del exe, que es donde están bin\, www\, etc.
const devBuild = false
