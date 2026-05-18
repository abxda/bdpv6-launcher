//go:build darwin
// +build darwin

package setup

import "syscall"

// macOS does not have a HideWindow concept; return nil so cmd.SysProcAttr
// stays unset and the runtime uses defaults.
func hideWindowAttr() *syscall.SysProcAttr {
	return nil
}
