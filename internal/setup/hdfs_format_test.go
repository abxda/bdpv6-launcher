package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abxda/bdpv6-launcher/internal/logsink"
	"github.com/abxda/bdpv6-launcher/internal/paths"
	"github.com/abxda/bdpv6-launcher/internal/state"
)

// TestCleanupBeforeFormat reproduces the corrupted-state scenario the user
// hit (in_use.lock orphan + current/ with edits but no VERSION + datanode
// from a previous clusterID) and asserts our cleanup removes everything
// so that the subsequent `hdfs namenode -format` will start from a clean
// slate.
func TestCleanupBeforeFormat(t *testing.T) {
	tmp := t.TempDir()
	p := &paths.Paths{
		ScriptDir:  tmp,
		Data:       filepath.Join(tmp, "data"),
		Hadoop:     filepath.Join(tmp, "hadoop"),
		HadoopConf: filepath.Join(tmp, "hadoop", "etc", "hadoop"),
		Kafka:      filepath.Join(tmp, "kafka_kraft"),
		Logs:       filepath.Join(tmp, "logs"),
	}

	// Build the broken state matching the user's incident:
	nnRoot := filepath.Join(p.Data, "hdfs", "namenode")
	nnCurr := filepath.Join(nnRoot, "current")
	dnRoot := filepath.Join(p.Data, "hdfs", "datanode")
	dnCurr := filepath.Join(dnRoot, "current")
	if err := os.MkdirAll(nnCurr, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dnCurr, 0o755); err != nil {
		t.Fatal(err)
	}
	must := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(nnRoot, "in_use.lock"), "stale pid")
	must(filepath.Join(dnRoot, "in_use.lock"), "stale pid")
	must(filepath.Join(nnCurr, "edits_inprogress_0000000000000000002"), "garbage edits")
	must(filepath.Join(dnCurr, "VERSION"), "datanode from previous clusterID")

	// Sanity: this state must classify as corrupted before cleanup.
	if got := p.NamenodeStateOf(); got != paths.NamenodeCorruptedSt {
		t.Fatalf("setup precondition: state %q, want %q", got, paths.NamenodeCorruptedSt)
	}

	sink := logsink.New("cleanup_test", "", 100)
	defer sink.Close()
	o := New(p, state.NewStore(filepath.Join(tmp, ".bdp_state.json")), sink)
	o.cleanupBeforeFormat()

	// Post: state classifies as empty (eligible for fresh format).
	if got := p.NamenodeStateOf(); got != paths.NamenodeEmpty {
		t.Errorf("post cleanup: state %q, want %q", got, paths.NamenodeEmpty)
	}
	// in_use.lock should be gone.
	for _, lock := range []string{
		filepath.Join(nnRoot, "in_use.lock"),
		filepath.Join(dnRoot, "in_use.lock"),
	} {
		if _, err := os.Stat(lock); !os.IsNotExist(err) {
			t.Errorf("lock should be gone: %s", lock)
		}
	}
	// datanode dir should be wiped.
	if _, err := os.Stat(dnRoot); !os.IsNotExist(err) {
		t.Errorf("datanode dir should be gone: %s", dnRoot)
	}
}
