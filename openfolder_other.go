//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

// openFolder abre una carpeta en el gestor de archivos del SO (macOS / Linux).
func openFolder(path string) error {
	bin := "xdg-open"
	if runtime.GOOS == "darwin" {
		bin = "open"
	}
	return exec.Command(bin, path).Start()
}
