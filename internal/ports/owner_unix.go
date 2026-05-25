//go:build darwin || linux
// +build darwin linux

package ports

import (
	"os/exec"
	"strconv"
	"strings"
)

// WhoOwns uses `lsof -nP -iTCP:PORT -sTCP:LISTEN -F pc` to identify the
// listener. Output is line-prefixed: 'p' = pid, 'c' = command. lsof exists
// and accepts the same flags on both macOS and Linux. On Linux distros
// where lsof isn't bundled (a few minimal images), the user can install
// it with apt/dnf/pacman; we degrade to an empty Owner rather than crash.
func WhoOwns(port int) Owner {
	cmd := exec.Command("lsof", "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-F", "pc")
	out, err := cmd.Output()
	if err != nil {
		return Owner{}
	}
	owner := Owner{}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			if pid, err := strconv.Atoi(line[1:]); err == nil {
				owner.PID = pid
			}
		case 'c':
			owner.Name = line[1:]
		}
	}
	return owner
}
