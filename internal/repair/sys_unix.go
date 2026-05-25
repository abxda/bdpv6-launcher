//go:build darwin || linux
// +build darwin linux

package repair

import "syscall"

// Unix has no "hide window" concept for forked processes; return nil so
// SysProcAttr stays at its zero value and Go uses defaults.
func hideWindowAttr() *syscall.SysProcAttr { return nil }
