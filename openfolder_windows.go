//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// openFolder abre una carpeta en el Explorador de Windows SIN abrir una ventana
// de consola (HideWindow), consistente con el resto del launcher. explorer.exe
// devuelve códigos de salida no-cero aun con éxito, así que no tratamos el
// error de Start como fallo.
func openFolder(path string) error {
	cmd := exec.Command("explorer", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
	return nil
}
