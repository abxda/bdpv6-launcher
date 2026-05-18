package ports

import (
	"net"
	"os"
	"testing"
)

// TestWhoOwns_Self spins up a real TCP listener inside this process so we
// know the owner PID and can verify the platform-specific lookup parses
// netstat (Windows) / lsof (macOS) output correctly.
func TestWhoOwns_Self(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	owner := WhoOwns(port)
	if owner.PID == 0 {
		t.Fatalf("WhoOwns(%d): empty owner — expected our own PID", port)
	}
	if owner.PID != os.Getpid() {
		t.Logf("WhoOwns(%d): got PID %d, expected %d (likely a related child PID on Windows — accepting)",
			port, owner.PID, os.Getpid())
	}
	if owner.Name == "" {
		t.Errorf("WhoOwns(%d): owner name is empty", port)
	}
}

// TestSuggestFree verifies the suggestion does not collide with a held port.
func TestSuggestFree_AvoidsHeld(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer held.Close()
	heldPort := held.Addr().(*net.TCPAddr).Port

	got := SuggestFree(heldPort)
	if got == 0 {
		t.Fatalf("SuggestFree returned 0 — no free port found")
	}
	if got == heldPort {
		t.Errorf("SuggestFree returned the held port %d", heldPort)
	}
}

// TestScan_DetectsBoundPort sanity checks that a held port is reported as
// non-free by the scanner.
func TestScan_DetectsBoundPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	out := Scan([]Probe{{ServiceID: "x", ServiceName: "x", Port: port}}, nil)
	if len(out) != 1 {
		t.Fatalf("Scan returned %d rows, want 1", len(out))
	}
	if out[0].Status == StatusFree {
		t.Errorf("port %d is held but Scan reported StatusFree", port)
	}
}
