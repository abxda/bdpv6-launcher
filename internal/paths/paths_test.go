package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestUnwrapAppBundle verifies the macOS .app bundle unwrap: a binary at
// <root>/<Name>.app/Contents/MacOS/<bin> must resolve its distro root to
// <root> (the parent of the .app), not to Contents/MacOS/.
func TestUnwrapAppBundle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("bundle unwrap only applies on darwin")
	}
	root := filepath.FromSlash("/Volumes/Drive/BDPV5_macOS")
	cases := map[string]string{
		// Inside a .app bundle (note the space in the product name): unwrap.
		filepath.Join(root, "BDPV6 Launcher.app", "Contents", "MacOS"): root,
		// Plain binary sitting at the distro root (e.g. go test): unchanged.
		root: root,
		// A MacOS dir not inside Contents/*.app: unchanged.
		filepath.Join(root, "MacOS"): filepath.Join(root, "MacOS"),
	}
	for in, want := range cases {
		if got := unwrapAppBundle(in); got != want {
			t.Errorf("unwrapAppBundle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNamenodeStateOf(t *testing.T) {
	tmp := t.TempDir()
	p := &Paths{Data: filepath.Join(tmp, "data"), Kafka: filepath.Join(tmp, "kafka_kraft")}

	// 1. Empty: no current/ at all → NamenodeEmpty.
	if got := p.NamenodeStateOf(); got != NamenodeEmpty {
		t.Errorf("empty: got %q, want %q", got, NamenodeEmpty)
	}

	// 2. Formatted: current/VERSION present → NamenodeFormattedOK.
	curr := filepath.Join(p.Data, "hdfs", "namenode", "current")
	if err := os.MkdirAll(curr, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(curr, "VERSION"), []byte("namespaceID=1\nclusterID=x\n"), 0o644); err != nil {
		t.Fatalf("WriteFile VERSION: %v", err)
	}
	if got := p.NamenodeStateOf(); got != NamenodeFormattedOK {
		t.Errorf("formatted: got %q, want %q", got, NamenodeFormattedOK)
	}

	// 3. Corrupted: current/ has stale files but no VERSION.
	_ = os.Remove(filepath.Join(curr, "VERSION"))
	if err := os.WriteFile(filepath.Join(curr, "edits_inprogress_0000000000000000002"), []byte("garbage"), 0o644); err != nil {
		t.Fatalf("WriteFile edits: %v", err)
	}
	if got := p.NamenodeStateOf(); got != NamenodeCorruptedSt {
		t.Errorf("corrupted: got %q, want %q", got, NamenodeCorruptedSt)
	}

	// 4. Empty current/ (mkdir but nothing inside) → NamenodeEmpty, not Corrupted.
	_ = os.RemoveAll(curr)
	if err := os.MkdirAll(curr, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got := p.NamenodeStateOf(); got != NamenodeEmpty {
		t.Errorf("empty current/: got %q, want %q", got, NamenodeEmpty)
	}
}

// TestNamenodeStateOf_RealDist asserts the user's actual on-disk state when
// BDP_DIST is set. Useful for diagnosing reports from the field.
func TestNamenodeStateOf_RealDist(t *testing.T) {
	dist := os.Getenv("BDP_DIST")
	if dist == "" {
		t.Skip("BDP_DIST not set")
	}
	p := &Paths{Data: filepath.Join(dist, "data"), Kafka: filepath.Join(dist, "kafka_kraft")}
	t.Logf("namenode state at %s: %s", dist, p.NamenodeStateOf())
	t.Logf("namenode formatted (legacy bool): %v", p.NamenodeFormatted())
	t.Logf("kafka formatted:                  %v", p.KafkaFormatted())
}
