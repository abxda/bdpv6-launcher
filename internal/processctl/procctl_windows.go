//go:build windows
// +build windows

package processctl

import (
	"os/exec"
	"strconv"
	"syscall"
)

// CREATE_NEW_PROCESS_GROUP detaches the child from this process's console
// group, which lets us later kill the whole tree without taking ourselves
// down with it. We hide the window so .bat/.cmd wrappers do not pop a black
// flash.
const (
	createNewProcessGroup = 0x00000200
)

func platformSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup,
		HideWindow:    true,
	}
}

// platformGracefulStop asks the process tree to terminate without /F. Java
// console apps tend to ignore this (they would only respond to a real
// Ctrl+Break), but the call is still cheap and works for many .bat wrappers.
// Returning an error just signals "try force next".
func platformGracefulStop(pid int) error {
	return runTaskkill(pid, false)
}

// platformForceStop kills the whole tree unconditionally with /F /T.
func platformForceStop(pid int) error {
	return runTaskkill(pid, true)
}

func runTaskkill(pid int, force bool) error {
	args := []string{"/T", "/PID", strconv.Itoa(pid)}
	if force {
		args = append([]string{"/F"}, args...)
	}
	cmd := exec.Command("taskkill", args...)
	// Avoid the console flash from taskkill itself.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}
