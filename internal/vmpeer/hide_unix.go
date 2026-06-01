//go:build darwin || linux

package vmpeer

import (
	"os"
	"os/exec"
)

func hideConsole(cmd *exec.Cmd) {}

func exists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
