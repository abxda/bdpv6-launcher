//go:build e2e
// +build e2e

package exercises

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abxda/bdpv6-launcher/internal/logsink"
	"github.com/abxda/bdpv6-launcher/internal/paths"
	"github.com/abxda/bdpv6-launcher/internal/ports"
	"github.com/abxda/bdpv6-launcher/internal/services"
)

// sweepPort is a defensive last-resort cleanup. If a service's Stop()
// didn't fully kill its JVM (Windows + Java can be quirky), the port
// stays bound by a zombie process. We find the owner and SIGKILL it so
// the next test (or the user's next launcher session) doesn't inherit
// "Address already in use" failures.
func sweepPort(t *testing.T, port int) {
	own := ports.WhoOwns(port)
	if own.PID == 0 {
		return // already free
	}
	if err := ports.KillByPID(own.PID); err != nil {
		t.Logf("sweepPort(%d): could not kill PID %d (%s): %v", port, own.PID, own.Name, err)
		return
	}
	t.Logf("sweepPort(%d): killed zombie PID %d (%s) holding the port", port, own.PID, own.Name)
}

// TestE2E_WordCountIdempotent is the gold-standard regression test: it
// spawns a real NameNode + DataNode, discovers Ejercicio_01, runs all 5
// steps of the WordCount playbook end-to-end, verifies the output ended
// up in HDFS with non-empty content, then re-runs ALL steps a second
// time to prove the playbook is idempotent (each step survives being
// re-executed against an already-populated HDFS without manual cleanup).
//
// Gated on BDP_DIST + e2e build tag. Total wall time ~90-120 s.
func TestE2E_WordCountIdempotent(t *testing.T) {
	dist := os.Getenv("BDP_DIST")
	if dist == "" {
		t.Skip("BDP_DIST not set")
	}
	p := buildE2EPaths(dist)

	// --- 1. Start HDFS NN + DN -----------------------------------------
	nn := services.NewHDFSNameNode(p, 9870)
	dn := services.NewHDFSDataNode(p, 9864)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := nn.Start(ctx); err != nil {
		t.Fatalf("nn Start: %v", err)
	}
	t.Cleanup(func() {
		c, cn := context.WithTimeout(context.Background(), 60*time.Second)
		defer cn()
		_ = nn.GracefulPreStop(c)
		_ = nn.Stop(c)
		// Defensive sweep: if the JVM somehow outlived Stop() (it has
		// happened — Java + taskkill is not always perfectly reliable on
		// Windows), kill anything still holding the NameNode ports so
		// the next test or the user's next launcher run does not
		// inherit zombies.
		sweepPort(t, 9870)
		sweepPort(t, 9000)
	})

	if err := waitHealthy(t, nn.Status, 60*time.Second, "namenode"); err != nil {
		t.Fatal(err)
	}

	if err := dn.Start(ctx); err != nil {
		t.Fatalf("dn Start: %v", err)
	}
	t.Cleanup(func() {
		c, cn := context.WithTimeout(context.Background(), 30*time.Second)
		defer cn()
		_ = dn.Stop(c)
		sweepPort(t, 9864)
		sweepPort(t, 9866)
	})
	if err := waitHealthy(t, dn.Status, 30*time.Second, "datanode"); err != nil {
		t.Fatal(err)
	}

	// --- 2. Discover Ejercicio_01 --------------------------------------
	exes := Discover(p)
	var ej01 *Exercise
	for i := range exes {
		if exes[i].ID == "ej01" {
			ej01 = &exes[i]
			break
		}
	}
	if ej01 == nil {
		t.Fatalf("Ejercicio_01 not discovered; got %d exercises", len(exes))
	}
	if ej01.Template != "wordcount" {
		t.Fatalf("ej01 template = %q, want wordcount", ej01.Template)
	}

	// --- 3. Run all 5 steps (first time) -------------------------------
	sink := logsink.New("e2e-ej01", filepath.Join(dist, "logs"), 5000)
	defer sink.Close()
	runner := NewRunner(*ej01, p, sink)

	runCtx, runCancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer runCancel()
	t.Logf("first run of the playbook...")
	if err := runner.RunAll(runCtx); err != nil {
		dumpTail(t, sink)
		t.Fatalf("first RunAll failed: %v", err)
	}

	// --- 4. Verify output via WebHDFS ----------------------------------
	if err := assertOutputExists(t, "/ej01/output"); err != nil {
		dumpTail(t, sink)
		t.Fatalf("first run output check: %v", err)
	}

	// --- 5. RE-RUN: prove idempotency ----------------------------------
	t.Logf("second run of the playbook (idempotency check)...")
	runCtx2, runCancel2 := context.WithTimeout(context.Background(), 180*time.Second)
	defer runCancel2()
	if err := runner.RunAll(runCtx2); err != nil {
		dumpTail(t, sink)
		t.Fatalf("second RunAll failed (NOT idempotent): %v", err)
	}
	if err := assertOutputExists(t, "/ej01/output"); err != nil {
		dumpTail(t, sink)
		t.Fatalf("second run output check: %v", err)
	}

	t.Logf("WordCount E2E: both runs succeeded — playbook is idempotent.")
}

// waitHealthy polls a service's Status until it flips healthy or the
// deadline elapses.
func waitHealthy(t *testing.T, status func() services.Status, budget time.Duration, label string) error {
	deadline := time.Now().Add(budget)
	for {
		s := status()
		if s.Running && s.Healthy {
			t.Logf("%s healthy after %s (PID=%d)", label, time.Since(s.Since).Round(time.Second), s.PID)
			return nil
		}
		if !s.Running {
			return fmt.Errorf("%s exited before becoming healthy", label)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s never healthy within %s", label, budget)
		}
		time.Sleep(2 * time.Second)
	}
}

// assertOutputExists hits WebHDFS to confirm /ej01/output/part-00000
// exists and has non-zero size.
func assertOutputExists(t *testing.T, hdfsPath string) error {
	url := fmt.Sprintf("http://127.0.0.1:9870/webhdfs/v1%s?op=LISTSTATUS", hdfsPath)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("webhdfs LISTSTATUS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("LISTSTATUS %s → HTTP %d", hdfsPath, resp.StatusCode)
	}
	var wrap struct {
		FileStatuses struct {
			FileStatus []struct {
				PathSuffix string `json:"pathSuffix"`
				Length     int64  `json:"length"`
				Type       string `json:"type"`
			} `json:"FileStatus"`
		} `json:"FileStatuses"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrap); err != nil {
		return err
	}
	for _, f := range wrap.FileStatuses.FileStatus {
		if f.PathSuffix == "part-00000" {
			if f.Length <= 0 {
				return fmt.Errorf("part-00000 exists but is empty")
			}
			t.Logf("output OK: %s/part-00000 = %d bytes", hdfsPath, f.Length)
			return nil
		}
	}
	return fmt.Errorf("part-00000 not found in %s; got %d files", hdfsPath, len(wrap.FileStatuses.FileStatus))
}

// dumpTail prints the last N log lines so failed runs show debugging context.
func dumpTail(t *testing.T, sink *logsink.Sink) {
	lines := sink.Snapshot()
	if len(lines) > 60 {
		lines = lines[len(lines)-60:]
	}
	t.Logf("---- last %d log lines ----", len(lines))
	for _, ln := range lines {
		t.Logf("  %s %s", ln.Level, ln.Text)
	}
}

// buildE2EPaths duplicates the helper from services/e2e_kafka_test.go
// (same signature, but in this package to avoid a cross-package import).
func buildE2EPaths(dist string) *paths.Paths {
	return &paths.Paths{
		ScriptDir:  dist,
		CommonJDK:  filepath.Join(dist, "common_jdk"),
		Hadoop:     filepath.Join(dist, "hadoop"),
		HadoopConf: filepath.Join(dist, "hadoop", "etc", "hadoop"),
		Kafka:      filepath.Join(dist, "kafka_kraft"),
		Elastic:    filepath.Join(dist, "elasticsearch"),
		Python:     filepath.Join(dist, "python"),
		Spark:      filepath.Join(dist, "spark"),
		Notebooks:  filepath.Join(dist, "notebooks"),
		Data:       filepath.Join(dist, "data"),
		Logs:       filepath.Join(dist, "logs"),
	}
}
