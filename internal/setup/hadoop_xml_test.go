package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abxda/bdpv6-launcher/internal/paths"
)

// TestWriteHadoopXML_RegeneratesWithCorrectURI is the regression test for
// the long-standing bug where BDPV5_macOS shipped core-site.xml with a
// path baked in to /Users/Sl4sh3r99/.... We confirm that running setup
// against any arbitrary SCRIPT_DIR produces XMLs whose URI matches that
// dir verbatim and never contains the stale Sl4sh3r99 token.
func TestWriteHadoopXML_RegeneratesWithCorrectURI(t *testing.T) {
	tmp := t.TempDir()
	conf := filepath.Join(tmp, "hadoop", "etc", "hadoop")
	if err := os.MkdirAll(conf, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	p := &paths.Paths{
		ScriptDir:  tmp,
		Hadoop:     filepath.Join(tmp, "hadoop"),
		HadoopConf: conf,
	}
	if err := WriteHadoopXML(p); err != nil {
		t.Fatalf("WriteHadoopXML: %v", err)
	}
	core, err := os.ReadFile(p.CoreSiteXML())
	if err != nil {
		t.Fatalf("read core-site.xml: %v", err)
	}
	hdfs, err := os.ReadFile(p.HdfsSiteXML())
	if err != nil {
		t.Fatalf("read hdfs-site.xml: %v", err)
	}
	for _, contents := range [][]byte{core, hdfs} {
		s := string(contents)
		if strings.Contains(s, "Sl4sh3r99") {
			t.Errorf("regenerated XML still contains the stale Sl4sh3r99 path:\n%s", s)
		}
		// Each XML should reference the SCRIPT_DIR-derived URI.
		want := uriPath(tmp)
		if !strings.Contains(s, want) {
			t.Errorf("regenerated XML missing expected URI %q:\n%s", want, s)
		}
	}
}

func TestURIPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{`D:\BDP\BDPV4_WIN`, "file:///D:/BDP/BDPV4_WIN"},
		{`/Users/abel/BDP`, "file:///Users/abel/BDP"},
		{`C:\path with spaces\bdp`, "file:///C:/path with spaces/bdp"},
	}
	for _, c := range cases {
		if got := uriPath(c.in); got != c.want {
			t.Errorf("uriPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
