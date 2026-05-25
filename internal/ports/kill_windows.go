//go:build windows
// +build windows

package ports

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

func killByPID(pid int) error {
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("taskkill PID %d: %v — %s", pid, err, string(out))
	}
	return nil
}
