//go:build darwin || linux
// +build darwin linux

package ports

import "syscall"

func killByPID(pid int) error {
	// Negative pid → process group. We don't know the group here, so just
	// send SIGKILL to the pid; if the user's intent is to "free this port"
	// they want the listener gone, not necessarily the whole tree.
	return syscall.Kill(pid, syscall.SIGKILL)
}
