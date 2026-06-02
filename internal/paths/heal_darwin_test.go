//go:build darwin

package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeDistro builds the minimal layout HealDarwin's probes look at, with
// the exact pathologies a fresh exFAT/zip extract leaves behind: service
// binaries with no exec bit and a versioned libsqlite3 but no unversioned
// alias.
func fakeDistro(t *testing.T) *Paths {
	t.Helper()
	tmp := t.TempDir()
	mk := func(rel string, mode os.FileMode, content string) {
		full := filepath.Join(tmp, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	// Service probes — mode 0o644 means "exec bit lost", which is the
	// signal HealDarwin uses to trigger the per-tree chmod.
	mk("common_jdk/bin/java", 0o644, "")
	mk("hadoop/bin/hdfs", 0o644, "")
	mk("kafka_kraft/bin/kafka-server-start.sh", 0o644, "")
	mk("elasticsearch/bin/elasticsearch", 0o644, "")
	// ES ships its own bundled JDK + native CLIs outside bin/. These must
	// also be re-chmod'd, or `java` under jdk/ stays non-exec and ES won't
	// start. The exec-bit heal keys off elasticsearch/bin/elasticsearch.
	mk("elasticsearch/jdk/Contents/Home/bin/java", 0o644, "")
	mk("elasticsearch/modules/x-pack-ml/platform/darwin-aarch64/bin/pytorch_inference", 0o644, "")
	// python3.10 must already be executable for the wrapper heal to fire
	// — that's the real-world state (chmod survives once set; the wrapper
	// files themselves are what the exFAT copy drops).
	mk("python/bin/python3.10", 0o755, "")
	mk("spark/bin/spark-submit", 0o644, "")
	// Versioned libsqlite3 only, no unversioned: simulates dropped symlink.
	mk("python/lib/libsqlite3.3.50.1.dylib", 0o644, "stub")

	return &Paths{
		ScriptDir: tmp,
		CommonJDK: filepath.Join(tmp, "common_jdk"),
		Hadoop:    filepath.Join(tmp, "hadoop"),
		Kafka:     filepath.Join(tmp, "kafka_kraft"),
		Elastic:   filepath.Join(tmp, "elasticsearch"),
		Python:    filepath.Join(tmp, "python"),
		Spark:     filepath.Join(tmp, "spark"),
	}
}

// TestHeal_FreshlyExtractedFromExFAT exercises every heal step from the
// state a student would see right after extracting a damaged tar/zip:
// missing exec bits, missing python shims, missing libsqlite3.dylib alias.
func TestHeal_FreshlyExtractedFromExFAT(t *testing.T) {
	p := fakeDistro(t)
	rep := p.HealDarwin()

	if len(rep.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", rep.Errors)
	}

	for _, name := range []string{"python", "python3"} {
		shim := filepath.Join(p.Python, "bin", name)
		fi, err := os.Stat(shim)
		if err != nil {
			t.Fatalf("missing wrapper %s: %v", name, err)
		}
		if fi.Mode()&0o100 == 0 {
			t.Errorf("wrapper %s not executable: mode=%o", name, fi.Mode())
		}
	}

	if _, err := os.Stat(filepath.Join(p.Python, "lib", "libsqlite3.dylib")); err != nil {
		t.Errorf("libsqlite3.dylib not restored: %v", err)
	}

	for _, bin := range []string{
		filepath.Join(p.CommonJDK, "bin", "java"),
		filepath.Join(p.Hadoop, "bin", "hdfs"),
		filepath.Join(p.Kafka, "bin", "kafka-server-start.sh"),
		filepath.Join(p.Elastic, "bin", "elasticsearch"),
		// ES bundled JDK + native module CLI must be healed too (the gap
		// found testing ES startup from exFAT).
		filepath.Join(p.Elastic, "jdk", "Contents", "Home", "bin", "java"),
		filepath.Join(p.Elastic, "modules", "x-pack-ml", "platform", "darwin-aarch64", "bin", "pytorch_inference"),
		filepath.Join(p.Spark, "bin", "spark-submit"),
	} {
		fi, err := os.Stat(bin)
		if err != nil {
			t.Fatalf("stat %s: %v", bin, err)
		}
		if fi.Mode()&0o100 == 0 {
			t.Errorf("not executable after heal: %s mode=%o", bin, fi.Mode())
		}
	}
}

// TestHeal_IdempotentOnHealthyDistro asserts that the second call is a
// pure no-op: HealDarwin runs on every startup, so an unnecessary action
// would mean we're modifying the distro on each boot (a real bug — slow
// on USB and noisy in logs).
func TestHeal_IdempotentOnHealthyDistro(t *testing.T) {
	p := fakeDistro(t)
	_ = p.HealDarwin() // first heal repairs everything

	rep := p.HealDarwin() // second run must do nothing
	if len(rep.Actions) != 0 {
		t.Errorf("expected no actions on healthy distro, got: %v", rep.Actions)
	}
	if len(rep.Errors) != 0 {
		t.Errorf("expected no errors on healthy distro, got: %v", rep.Errors)
	}
}
