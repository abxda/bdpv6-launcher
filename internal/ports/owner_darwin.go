//go:build darwin
// +build darwin

package ports

import (
	"os/exec"
	"strconv"
	"strings"
)

// WhoOwns uses `lsof -nP -iTCP:PORT -sTCP:LISTEN -F pcn` to identify the
// listener. Output is line-prefixed: 'p' = pid, 'c' = command, 'n' = name.
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
