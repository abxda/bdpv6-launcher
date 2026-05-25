//go:build darwin || linux
// +build darwin linux

package processctl

import "syscall"

// Setpgid makes the child the leader of its own process group, so SIGTERM /
// SIGKILL can be sent to -pgid and reach grandchildren too (Java spawned by
// a wrapper shell script). pgid == pid for a fresh leader. Identical
// semantics on Darwin and Linux — both implement POSIX process groups.
func platformSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// platformGracefulStop sends SIGTERM to the process group. On the JVM this
// triggers Runtime.getRuntime().addShutdownHook() handlers — which for
// Hadoop, Kafka, and Elasticsearch include flushing in-memory state to
// disk. Much cleaner than the Windows side, where Java console apps
// ignore taskkill /T entirely.
func platformGracefulStop(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}

func platformForceStop(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
