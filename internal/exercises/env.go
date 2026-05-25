package exercises

import (
	"os"
	"runtime"
	"strings"
)

// mergeOSEnv composes os.Environ() with override map. Later writes win.
func mergeOSEnv(overrides map[string]string) []string {
	out := []string{}
	used := map[string]bool{}
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			out = append(out, kv)
			continue
		}
		k := kv[:eq]
		if v, ok := overrides[k]; ok {
			out = append(out, k+"="+v)
			used[k] = true
		} else {
			out = append(out, kv)
		}
	}
	for k, v := range overrides {
		if !used[k] {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// mergePath prepends the given directories to the current PATH so they
// take precedence over anything the user may already have installed
// (a system-wide Java 17, a different Python, etc).
func mergePath(prepend []string) string {
	sep := ":"
	if runtime.GOOS == "windows" {
		sep = ";"
	}
	parts := append([]string{}, prepend...)
	if current := os.Getenv("PATH"); current != "" {
		parts = append(parts, current)
	}
	return strings.Join(parts, sep)
}
