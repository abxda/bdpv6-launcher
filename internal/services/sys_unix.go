//go:build darwin || linux
// +build darwin linux

package services

import "syscall"

// Unix platforms have no "hide window" concept for forked processes; return
// nil so SysProcAttr stays at its zero value and Go uses defaults.
func hideWindowSysAttr() *syscall.SysProcAttr { return nil }
