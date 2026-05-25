//go:build darwin
// +build darwin

package services

import "syscall"

// macOS has no "hide window" concept for forked processes; return nil so
// SysProcAttr stays at its zero value and Go uses defaults.
func hideWindowSysAttr() *syscall.SysProcAttr { return nil }
