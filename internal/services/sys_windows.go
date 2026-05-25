//go:build windows
// +build windows

package services

import "syscall"

// hideWindowSysAttr returns a SysProcAttr that prevents a console window
// from flashing when one-shot helper commands (like `hdfs.cmd dfsadmin`)
// are invoked. Long-running services already get a similar attr from
// processctl_windows.go; this helper is for synchronous Cmd-style calls
// that bypass processctl.
func hideWindowSysAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}
