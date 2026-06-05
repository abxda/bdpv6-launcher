//go:build windows

package main

import "os/exec"

// openFolder abre una carpeta en el Explorador de Windows.
// OJO: NO uses HideWindow/CREATE_NO_WINDOW con explorer.exe — esa bandera impide
// que el Explorador abra la ventana. explorer es app GUI, no abre consola, así
// que no hay nada que ocultar. Devuelve exit-code no-cero aun con éxito, por eso
// Start (no Run) y solo propagamos el fallo de lanzar el proceso.
func openFolder(path string) error {
	return exec.Command("explorer", path).Start()
}
