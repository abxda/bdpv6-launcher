package exercises

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/abxda/bdpv6-launcher/internal/paths"
)

// resolveBash returns the absolute path to a bash binary suitable for
// running shell-piped steps. Resolution order:
//
//  1. <ScriptDir>/bash/usr/bin/bash.exe (Windows, bundled GitPortable)
//  2. <ScriptDir>/bash/bin/bash         (Unix-ish layout, just in case)
//  3. `bash` on the system PATH         (Mac / Linux)
//  4. "" if nothing found — caller can mark shell steps unavailable.
func resolveBash(p *paths.Paths) string {
	candidates := []string{
		filepath.Join(p.ScriptDir, "bash", "usr", "bin", "bash.exe"),
		filepath.Join(p.ScriptDir, "bash", "bin", "bash.exe"),
		filepath.Join(p.ScriptDir, "bash", "bin", "bash"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if found, err := exec.LookPath("bash"); err == nil {
		return found
	}
	return ""
}
