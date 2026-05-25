//go:build darwin || linux
// +build darwin linux

package exercises

import "syscall"

// On Unix the "spawn a new terminal window" pattern is OS-specific and
// out of scope here — students on Mac/Linux already have a terminal and
// can open it themselves. The function exists so the cross-platform
// build succeeds; the App bound method will surface an empty-bash error.
func bashLaunchSysAttr() *syscall.SysProcAttr { return nil }
