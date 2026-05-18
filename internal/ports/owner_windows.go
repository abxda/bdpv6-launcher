//go:build windows
// +build windows

package ports

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// WhoOwns parses `netstat -ano -p tcp` looking for a LISTENING entry on
// :PORT, then asks tasklist for the friendly name of the owning PID. All
// errors degrade to a zero Owner — the UI just shows "ocupado".
func WhoOwns(port int) Owner {
	cmd := exec.Command("netstat", "-ano", "-p", "tcp")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return Owner{}
	}
	suffix := fmt.Sprintf(":%d", port)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		// Expect: Proto Local-Addr Foreign-Addr State PID
		if len(fields) < 5 {
			continue
		}
		state := fields[3]
		local := fields[1]
		if state != "LISTENING" {
			continue
		}
		if !strings.HasSuffix(local, suffix) {
			continue
		}
		pid, err := strconv.Atoi(fields[4])
		if err != nil {
			continue
		}
		return Owner{PID: pid, Name: processNameFor(pid)}
	}
	return Owner{}
}

func processNameFor(pid int) string {
	cmd := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// CSV: "ImageName","PID","SessionName","Session#","MemUsage"
	line := strings.TrimSpace(string(out))
	if line == "" || strings.HasPrefix(line, "INFO:") {
		return ""
	}
	first := strings.Split(line, ",")[0]
	return strings.Trim(first, "\"")
}
