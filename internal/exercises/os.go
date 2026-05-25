package exercises

import "runtime"

// isWindows is broken out so tests can stub it. Today it's a straight
// runtime.GOOS check.
func isWindows() bool { return runtime.GOOS == "windows" }
