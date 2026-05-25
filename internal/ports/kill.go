package ports

// KillByPID terminates the process tree rooted at pid. Cross-platform:
// taskkill /F /T on Windows, SIGKILL on Unix. Returns an error wrapping
// the underlying tool's output so callers can surface it in the UI.
//
// The actual platform-specific implementation lives in kill_windows.go /
// kill_unix.go so each side can use the most direct approach without
// build-tag gymnastics in the consumer code.
func KillByPID(pid int) error { return killByPID(pid) }
