//go:build windows
// +build windows

package exercises

import "syscall"

// bashLaunchSysAttr hides the cmd.exe stub window that briefly appears
// when we spawn `cmd /c start ... git-bash.exe`. The git-bash window
// itself is independent and visible — only the launcher stub is hidden.
func bashLaunchSysAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}
