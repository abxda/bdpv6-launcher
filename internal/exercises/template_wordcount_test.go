package exercises

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/abxda/bdpv6-launcher/internal/paths"
)

// TestStreamingStep_NoFilesNoComma is the regression test for the field
// bug where step 4 generated `-files file:///D:/.../mapper.py,file:///D:/.../reducer.py`.
// That blew up twice on Windows: first Hadoop's GenericOptionsParser
// choked on the backslashes ("Illegal character in opaque part at index 2"),
// then with file:/// URIs it survived URI validation but hadoop.cmd's
// internal FOR loops split on the comma and dropped the second URI
// ("Found 1 unexpected arguments [file:///.../reducer.py]").
//
// For single-node localhost (the BDP setup) -files is unnecessary
// anyway because the "worker" runs on the same machine as the submitter
// and can read the .py files directly. The fix is to pass absolute
// forward-slash paths into the -mapper / -reducer command strings and
// skip -files entirely.
func TestStreamingStep_NoFilesNoComma(t *testing.T) {
	tmp := t.TempDir()
	distro := filepath.Join(tmp, "distro")
	dir := filepath.Join(tmp, "Ejercicio_01")
	_ = mkdirs(distro, dir)
	for _, f := range []string{"mapper.py", "reducer.py", "data.csv"} {
		_ = touch(filepath.Join(dir, f))
	}
	p := &paths.Paths{
		ScriptDir: distro,
		Hadoop:    filepath.Join(distro, "hadoop"),
	}
	ex := wordCountExercise(p, "ej01", "Ejercicio_01", dir, []string{"data.csv", "mapper.py", "reducer.py"})
	if ex == nil || len(ex.Steps) < 5 {
		t.Fatal("template did not produce step 5 (the streaming job)")
	}
	// Step 5 in the new layout is the streaming job (step 1 is safe-mode-wait).
	step := ex.Steps[4]

	// Step 5 is now a bash -lc invocation that chains an idempotent
	// pre-clean ("hdfs dfs -rm ...") and the actual streaming job. The
	// full command string lives in step.Args[1]. We assert on its
	// contents instead of the old "are -mapper / -reducer separate
	// args" check.
	if !step.Shell {
		t.Error("step 5 is no longer marked Shell:true — that breaks the bash wrap")
	}
	if len(step.Args) < 2 || step.Args[0] != "-lc" {
		t.Fatalf("step 5 args = %#v, want [-lc <script>]", step.Args)
	}
	script := step.Args[1]

	if strings.Contains(script, "-files") {
		t.Error("step 5 still has -files; that broke on hadoop.cmd's comma re-tokenization")
	}
	if !strings.Contains(script, "mapper.py") {
		t.Error("step 5 script does not reference mapper.py")
	}
	if !strings.Contains(script, "reducer.py") {
		t.Error("step 5 script does not reference reducer.py")
	}
	if !strings.Contains(script, "hdfs") || !strings.Contains(script, "-rm") {
		t.Error("step 5 script lost the idempotent pre-clean (rm output)")
	}
	if !strings.Contains(script, "framework.name=local") {
		t.Error("step 5 script must force LocalJobRunner; -Dmapreduce.framework.name=local missing")
	}
}

func mkdirs(paths ...string) error {
	for _, p := range paths {
		if err := osMkdirAll(p); err != nil {
			return err
		}
	}
	return nil
}

func osMkdirAll(p string) error { return osStub.MkdirAll(p) }
func touch(p string) error     { return osStub.WriteFile(p, []byte("x")) }

// tiny indirection so the test file doesn't need to import os directly
// twice; keeps top-of-file imports clean.
var osStub = struct {
	MkdirAll  func(string) error
	WriteFile func(string, []byte) error
}{
	MkdirAll:  func(p string) error { return osMkdir(p) },
	WriteFile: func(p string, b []byte) error { return osWrite(p, b) },
}

// TestToFileURI is the regression test for the Hadoop streaming -files
// crash:
//
//	java.net.URISyntaxException: Illegal character in opaque part at
//	  index 2: D:\BDP\Ejercicio_01\mapper.py
//
// Hadoop's GenericOptionsParser feeds each comma-separated entry to
// `new URI(s)`. Without the file:/// scheme prefix and forward slashes,
// "D:\BDP\..." is parsed as scheme="D" + opaque part="\BDP\..." and the
// first backslash (index 2) is illegal in an opaque URI.
func TestToFileURI(t *testing.T) {
	cases := []struct{ in, want string }{
		{`D:\BDP\Ejercicio_01\mapper.py`, "file:///D:/BDP/Ejercicio_01/mapper.py"},
		{`C:\path with spaces\foo.py`, "file:///C:/path with spaces/foo.py"},
		{`/home/abel/ej01/mapper.py`, "file:///home/abel/ej01/mapper.py"},
		{`/Users/abel/BDP/Ejercicio_01/reducer.py`, "file:///Users/abel/BDP/Ejercicio_01/reducer.py"},
	}
	for _, c := range cases {
		if got := toFileURI(c.in); got != c.want {
			t.Errorf("toFileURI(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
