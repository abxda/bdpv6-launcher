//go:build e2e
// +build e2e

// E2E tests against the real BDP distribution. These actually spawn Java
// services and assume a populated BDP layout is reachable at the path
// pointed to by the BDP_DIST env var.
//
// Skipped by default. Run with:
//
//   BDP_DIST=D:\BDP\BDPV4_WIN go test -tags=e2e ./internal/services -run TestE2E_Elasticsearch -v -timeout 180s
//
// The Elasticsearch test is the safest: a single binary, no pre-format
// required, healthy in ~30 s on a warm machine.
package services

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/abxda/bdpv6-launcher/internal/paths"
)

func TestE2E_Elasticsearch(t *testing.T) {
	dist := os.Getenv("BDP_DIST")
	if dist == "" {
		t.Skip("BDP_DIST not set — set to a directory holding a populated BDP distribution to run this test")
	}
	p := &paths.Paths{
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
	// Sanity check before spending 30 s on a doomed run.
	if _, err := os.Stat(p.ElasticBinary()); err != nil {
		t.Fatalf("ES binary not found at %s: %v (set BDP_DIST correctly)", p.ElasticBinary(), err)
	}

	es := NewElasticsearch(p, 9200, "512m")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	t.Logf("starting Elasticsearch on platform %s/%s", runtime.GOOS, runtime.GOARCH)
	if err := es.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		if err := es.Stop(stopCtx); err != nil {
			t.Logf("Stop returned error: %v (process may have died on its own)", err)
		}
	})

	// Wait up to 90 s for the cluster health endpoint to return 200.
	deadline := time.Now().Add(90 * time.Second)
	for {
		st := es.Status()
		if st.Running && st.Healthy {
			t.Logf("Elasticsearch became healthy after %s, PID=%d", time.Since(st.Since).Round(time.Second), st.PID)
			return
		}
		if !st.Running {
			t.Fatalf("Elasticsearch process exited before becoming healthy (exit=%d, detail=%s)", st.ExitCode, st.Detail)
		}
		if time.Now().After(deadline) {
			lines := es.Logs().Snapshot()
			tail := lines
			if len(tail) > 30 {
				tail = tail[len(tail)-30:]
			}
			for _, ln := range tail {
				t.Logf("  log: %s", ln.Text)
			}
			t.Fatalf("Elasticsearch did not become healthy within deadline (running=%v, detail=%s)", st.Running, st.Detail)
		}
		time.Sleep(2 * time.Second)
	}
}
