//go:build darwin
// +build darwin

package processctl

import "syscall"

// Setpgid makes the child the leader of its own process group, so SIGTERM /
// SIGKILL can be sent to -pgid and reach grandchildren too (Java spawned by
// a wrapper shell script). pgid == pid for a fresh leader.
func platformSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func platformGracefulStop(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}

func platformForceStop(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
