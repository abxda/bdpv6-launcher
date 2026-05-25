//go:build e2e
// +build e2e

package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abxda/bdpv6-launcher/internal/logsink"
	"github.com/abxda/bdpv6-launcher/internal/paths"
)

// randomTestUUID returns a fresh 22-char base64url cluster id per run, so
// tests against a real BDP distribution do not bake a fixed "TestE2E_*"
// string into the user's Kafka metadata.
//
// First char is guaranteed alphanumeric: kafka-storage's argparse treats
// values starting with "-" or "_" as new flags, not as the value of -t.
func randomTestUUID(t *testing.T) string {
	for i := 0; i < 8; i++ {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			t.Fatalf("rand.Read: %v", err)
		}
		s := strings.TrimRight(base64.URLEncoding.EncodeToString(b[:]), "=")
		if len(s) > 0 && s[0] != '-' && s[0] != '_' {
			return s
		}
	}
	t.Fatalf("could not generate a safe UUID after 8 tries")
	return ""
}

// TestE2E_KafkaFormat regression-tests the bypass of kafka-storage.bat.
// Before the fix, the .bat overflowed cmd.exe's 8191-char limit when its
// CLASSPATH expansion + the config path was too long, and died with
// "La sintaxis del comando no es correcta." This test runs the same code
// path as setup.RunStep("kafka") would, asserting the storage tool exits
// 0 — proof that the wildcard-classpath workaround works end-to-end.
func TestE2E_KafkaFormat(t *testing.T) {
	dist := os.Getenv("BDP_DIST")
	if dist == "" {
		t.Skip("BDP_DIST not set")
	}
	p := buildE2EPaths(dist)

	// Make sure data_kraft is wiped. The kafka config has
	//   log.dirs=./data/data_kraft
	// which is resolved relative to the cwd we pass to java (= p.Kafka),
	// so the actual on-disk path is kafka_kraft/data/data_kraft, not
	// the top-level data/data_kraft.
	for _, dir := range []string{
		filepath.Join(p.Kafka, "data", "data_kraft"),
		filepath.Join(dist, "data", "data_kraft"),
	} {
		_ = os.RemoveAll(dir)
	}

	// Borrow the setup orchestrator's runKafka path by calling it through
	// a fake Orchestrator-equivalent: build the same java command inline.
	// We import the storage tool's exec path from the public Kafka helper
	// in this package and reproduce it here to keep the test self-contained.
	kafkaJar := filepath.Join(p.Kafka, "libs", "*")
	java := p.JavaBinary()
	if _, err := os.Stat(java); err != nil {
		t.Fatalf("java binary not found at %s: %v", java, err)
	}
	t.Logf("formatting Kafka with classpath=%s", kafkaJar)

	sink := logsink.New("e2e_kafka", filepath.Join(dist, "logs"), 500)
	defer sink.Close()

	// Reuse Kafka service Start by registering a fake config? Simpler:
	// shell out to java directly. This is exactly what the new runKafka
	// path does.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	uuid := randomTestUUID(t)
	t.Logf("using cluster id: %s", uuid)
	if err := runJavaStorageFormat(ctx, p, uuid); err != nil {
		t.Fatalf("kafka format failed: %v", err)
	}

	// Verify the meta.properties was created — proof the format took.
	meta := filepath.Join(p.Kafka, "data", "data_kraft", "meta.properties")
	if _, err := os.Stat(meta); err != nil {
		t.Fatalf("meta.properties not created at %s: %v", meta, err)
	}
	t.Logf("kafka format ok: %s", meta)
}

// TestE2E_KafkaStart is the second half of the Kafka bypass regression
// test: after a successful format, the broker actually comes up on 9092
// and stays up long enough to be probed before we tear it down.
func TestE2E_KafkaStart(t *testing.T) {
	dist := os.Getenv("BDP_DIST")
	if dist == "" {
		t.Skip("BDP_DIST not set")
	}
	p := buildE2EPaths(dist)

	k := NewKafka(p, 9092, "256m")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := k.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = k.Stop(stopCtx)
	})

	deadline := time.Now().Add(60 * time.Second)
	for {
		st := k.Status()
		if st.Running && st.Healthy {
			t.Logf("Kafka healthy after %s, PID=%d", time.Since(st.Since).Round(time.Second), st.PID)
			return
		}
		if !st.Running {
			lines := k.Logs().Snapshot()
			tail := lines
			if len(tail) > 25 {
				tail = tail[len(tail)-25:]
			}
			for _, ln := range tail {
				t.Logf("  log: %s", ln.Text)
			}
			t.Fatalf("Kafka exited before becoming healthy (exit=%d)", st.ExitCode)
		}
		if time.Now().After(deadline) {
			t.Fatalf("Kafka not healthy within deadline (running=%v, detail=%s)", st.Running, st.Detail)
		}
		time.Sleep(2 * time.Second)
	}
}

// runJavaStorageFormat duplicates the new setup.runKafka command shape so
// the test can prove the workaround end-to-end without depending on the
// setup package (which would create an import cycle).
func runJavaStorageFormat(ctx context.Context, p *paths.Paths, uuid string) error {
	classpath := filepath.Join(p.Kafka, "libs", "*")
	log4j := filepath.Join(p.Kafka, "config", "tools-log4j.properties")
	args := []string{
		"-Xms256m", "-Xmx256m",
		"-Dlog4j.configuration=file:" + filepath.ToSlash(log4j),
		"-classpath", classpath,
		"kafka.tools.StorageTool",
		"format", "-t", uuid, "-c", p.KafkaConfig(),
	}
	cmd := newJavaCmd(ctx, p, args)
	out, err := cmd.CombinedOutput()
	if testing.Verbose() {
		print(string(out))
	}
	return err
}

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
