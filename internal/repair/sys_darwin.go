//go:build darwin
// +build darwin

package repair

import "syscall"

func hideWindowAttr() *syscall.SysProcAttr {
	return nil
}
