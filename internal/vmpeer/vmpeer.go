// Package vmpeer detecta y apaga la VM de la Edición Vagrant ("el otro
// laboratorio") cuando ambas ediciones compiten por los mismos puertos. El
// Portable usa esto para ofrecer apagar la VM en vez de mostrar un error
// técnico de puerto ocupado por VBoxHeadless.
//
// Se controla por NOMBRE de VM (BDP-BigDataLab), igual que el panel Vagrant y
// el meta-launcher, así que funciona sin saber desde qué carpeta se levantó.
package vmpeer

import (
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// VMName es el nombre fijo que el Vagrantfile asigna a la VM del laboratorio.
const VMName = "BDP-BigDataLab"

// Running reporta si la VM de Vagrant está encendida (por nombre).
func Running() bool {
	out, err := vbox("list", "runningvms")
	if err != nil {
		return false
	}
	return strings.Contains(out, `"`+VMName+`"`)
}

// Shutdown apaga la VM por nombre con ACPI (apagado limpio del SO invitado).
// Espera hasta timeout; devuelve true si confirmó el apagado.
func Shutdown(timeout time.Duration) bool {
	if !Running() {
		return true
	}
	_, _ = vbox("controlvm", VMName, "acpipowerbutton")
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !Running() {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return !Running()
}

// OwnerIsVagrantVM reporta si el nombre de proceso dado corresponde a la VM de
// VirtualBox (VBoxHeadless / VirtualBoxVM). Lo usa el preflight de puertos para
// reconocer que el ocupante es "el otro laboratorio".
func OwnerIsVagrantVM(procName string) bool {
	n := strings.ToLower(procName)
	return strings.Contains(n, "vboxheadless") || strings.Contains(n, "virtualboxvm") || strings.Contains(n, "vboxsvc")
}

func vbox(args ...string) (string, error) {
	cmd := exec.Command(vboxBin(), args...)
	hideConsole(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func vboxBin() string {
	name := "VBoxManage"
	if runtime.GOOS == "windows" {
		name = "VBoxManage.exe"
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, c := range knownPaths() {
		if exists(c) {
			return c
		}
	}
	return name
}

func knownPaths() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{`C:\Program Files\Oracle\VirtualBox\VBoxManage.exe`}
	case "darwin":
		return []string{"/usr/local/bin/VBoxManage", "/opt/homebrew/bin/VBoxManage", "/Applications/VirtualBox.app/Contents/MacOS/VBoxManage"}
	default:
		return []string{"/usr/bin/VBoxManage", "/usr/local/bin/VBoxManage"}
	}
}
