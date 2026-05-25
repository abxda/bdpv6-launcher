//go:build darwin || linux
// +build darwin linux

package setup

import "syscall"

// Unix has no "hide window" concept for child processes; return nil so
// cmd.SysProcAttr stays at its zero value and Go uses defaults.
func hideWindowAttr() *syscall.SysProcAttr { return nil }
