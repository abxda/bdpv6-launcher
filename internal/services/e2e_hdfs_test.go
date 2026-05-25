//go:build e2e
// +build e2e

package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abxda/bdpv6-launcher/internal/paths"
	"github.com/abxda/bdpv6-launcher/internal/setup"
	"github.com/abxda/bdpv6-launcher/internal/state"
	"github.com/abxda/bdpv6-launcher/internal/logsink"
)

// TestE2E_HDFSGracefulShutdown is the regression test for the corruption
// loop the user hit before: hard-killing the NameNode leaves current/ with
// edits but no VERSION/fsimage, which classifies as "corrupted".
//
// With GracefulPreStop the dfsadmin -saveNamespace dance runs first, so
// after Stop the on-disk state classifies as "formatted" and a subsequent
// Start succeeds without auto-cleanup.
//
// Runs against a populated BDP distribution; gated on BDP_DIST. Spawns a
// real NameNode JVM for ~30 s.
func TestE2E_HDFSGracefulShutdown(t *testing.T) {
	dist := os.Getenv("BDP_DIST")
	if dist == "" {
		t.Skip("BDP_DIST not set")
	}
	p := buildE2EPaths(dist)

	// First, format (or reformat) so we start from a known-good state. We
	// drive the setup orchestrator the same way the wizard does.
	st := state.NewStore(filepath.Join(dist, ".bdp_state.json"))
	setupSink := logsink.New("e2e_hdfs_setup", filepath.Join(dist, "logs"), 500)
	defer setupSink.Close()
	orc := setup.New(p, st, setupSink)
	ctx0, cancel0 := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel0()
	if err := orc.RunStep(ctx0, "xml"); err != nil {
		t.Fatalf("xml step: %v", err)
	}
	if err := orc.RunStep(ctx0, "hdfs"); err != nil {
		t.Fatalf("hdfs format step: %v", err)
	}
	// Sanity: after format, state should be formatted (not empty, not corrupted).
	if got := p.NamenodeStateOf(); got != paths.NamenodeFormattedOK {
		t.Fatalf("after format: state=%q want %q", got, paths.NamenodeFormattedOK)
	}

	// Now start the namenode, wait for HTTP to come up, then graceful stop.
	nn := NewHDFSNameNode(p, 9870)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := nn.Start(ctx); err != nil {
		t.Fatalf("nn Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, c := context.WithTimeout(context.Background(), 60*time.Second)
		defer c()
		_ = nn.Stop(stopCtx)
	})

	// Wait up to 60 s for HTTP probe to pass.
	deadline := time.Now().Add(60 * time.Second)
	for {
		s := nn.Status()
		if s.Running && s.Healthy {
			t.Logf("namenode healthy after %s (PID=%d)", time.Since(s.Since).Round(time.Second), s.PID)
			break
		}
		if !s.Running {
			t.Fatalf("namenode exited before becoming healthy (exit=%d)", s.ExitCode)
		}
		if time.Now().After(deadline) {
			tail := nn.Logs().Snapshot()
			if len(tail) > 30 {
				tail = tail[len(tail)-30:]
			}
			for _, ln := range tail {
				t.Logf("  log: %s", ln.Text)
			}
			t.Fatalf("namenode never became healthy")
		}
		time.Sleep(2 * time.Second)
	}

	// THIS is what we're testing: GracefulPreStop forces a checkpoint
	// before the hard kill, so the on-disk state survives.
	preStopCtx, preCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer preCancel()
	if err := nn.GracefulPreStop(preStopCtx); err != nil {
		t.Fatalf("GracefulPreStop: %v", err)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()
	if err := nn.Stop(stopCtx); err != nil {
		t.Logf("Stop returned %v (may be benign on a process that exited cleanly)", err)
	}

	// Assertion: post-shutdown state must be "formatted", NOT "corrupted".
	// This proves the dfsadmin -saveNamespace wrote fsimage before kill.
	if got := p.NamenodeStateOf(); got != paths.NamenodeFormattedOK {
		t.Errorf("AFTER graceful shutdown: state=%q, want %q (corruption regression)", got, paths.NamenodeFormattedOK)

		// Dump the namenode dir contents to aid debugging.
		curr := filepath.Join(p.Data, "hdfs", "namenode", "current")
		if entries, err := os.ReadDir(curr); err == nil {
			for _, e := range entries {
				t.Logf("  current/%s", e.Name())
			}
		}
	} else {
		t.Logf("OK: namenode state remains 'formatted' after graceful shutdown")
	}

	// Lock file should not be present after a graceful shutdown.
	if _, err := os.Stat(filepath.Join(p.Data, "hdfs", "namenode", "in_use.lock")); err == nil {
		// On Windows the JVM holds in_use.lock as a file lock; it should
		// be released on shutdown. If it's still here as a regular file,
		// that's a minor leak but not catastrophic.
		t.Logf("note: in_use.lock still present (lock file may be a regular file on Windows)")
	}
}
