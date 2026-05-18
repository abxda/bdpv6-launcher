// Package ports answers two questions for the UI: "is this port free?" and
// "if not, who has it?". The Owner lookup is intentionally cheap (a single
// netstat / lsof call per scan) since the Ports tab refreshes on a timer.
//
// Platform-specific lookups live in owner_windows.go and owner_darwin.go.
package ports

import (
	"fmt"
	"net"
	"time"
)

// Status of a port from the launcher's perspective.
type Status string

const (
	StatusFree  Status = "free"
	StatusOurs  Status = "ours"  // a BDP service is bound here (we know its PID)
	StatusOther Status = "other" // someone else is bound (foreign PID)
)

// Owner describes the process currently bound to a local TCP port. Empty
// fields mean the lookup could not resolve the PID or process name.
type Owner struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
}

// Probe is one row in the Ports tab. ServiceID is non-empty when this port
// belongs to a known BDP service (so the UI can highlight "your" rows).
type Probe struct {
	ServiceID   string `json:"serviceId"`
	ServiceName string `json:"serviceName"`
	Port        int    `json:"port"`
	Status      Status `json:"status"`
	Owner       Owner  `json:"owner"`
}

// Scan returns one Probe per requested port. ourPIDs is the set of PIDs
// owned by BDP services (so we can tag StatusOurs vs StatusOther).
func Scan(probes []Probe, ourPIDs map[int]bool) []Probe {
	out := make([]Probe, len(probes))
	for i, p := range probes {
		out[i] = enrich(p, ourPIDs)
	}
	return out
}

func enrich(p Probe, ourPIDs map[int]bool) Probe {
	addr := fmt.Sprintf("127.0.0.1:%d", p.Port)
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		p.Status = StatusFree
		return p
	}
	_ = conn.Close()
	// Something is listening. Try to identify who.
	owner := WhoOwns(p.Port)
	p.Owner = owner
	if owner.PID > 0 && ourPIDs[owner.PID] {
		p.Status = StatusOurs
	} else {
		p.Status = StatusOther
	}
	return p
}

// SuggestFree returns the first free TCP port >= start, scanning up to a
// reasonable upper bound. Returns 0 if no free port could be found.
func SuggestFree(start int) int {
	if start < 1024 {
		start = 1024
	}
	for port := start; port < start+200 && port < 65535; port++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			continue
		}
		_ = ln.Close()
		return port
	}
	return 0
}
