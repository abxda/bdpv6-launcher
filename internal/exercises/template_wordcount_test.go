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
	if ex == nil || len(ex.Steps) < 4 {
		t.Fatal("template did not produce step 4")
	}
	step := ex.Steps[3]

	// Walk the args looking for -files (must NOT be present) and for the
	// -mapper / -reducer values (must contain forward-slash absolute paths
	// to mapper.py and reducer.py, no backslashes that would trip Hadoop's
	// command-string tokenizer when it interprets the value).
	var hasMapper, hasReducer bool
	for i, arg := range step.Args {
		if arg == "-files" {
			t.Errorf("step 4 still has -files; that broke on hadoop.cmd's comma re-tokenization")
		}
		if arg == "-mapper" && i+1 < len(step.Args) {
			v := step.Args[i+1]
			if !strings.Contains(v, "mapper.py") {
				t.Errorf("-mapper value %q does not reference mapper.py", v)
			}
			if strings.Contains(v, "\\") {
				t.Errorf("-mapper value %q has a backslash; use filepath.ToSlash", v)
			}
			hasMapper = true
		}
		if arg == "-reducer" && i+1 < len(step.Args) {
			v := step.Args[i+1]
			if !strings.Contains(v, "reducer.py") {
				t.Errorf("-reducer value %q does not reference reducer.py", v)
			}
			if strings.Contains(v, "\\") {
				t.Errorf("-reducer value %q has a backslash", v)
			}
			hasReducer = true
		}
	}
	if !hasMapper {
		t.Error("step 4 does not pass -mapper")
	}
	if !hasReducer {
		t.Error("step 4 does not pass -reducer")
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
