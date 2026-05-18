//go:build windows
// +build windows

package repair

import "syscall"

func hideWindowAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}
