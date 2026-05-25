package exercises

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abxda/bdpv6-launcher/internal/paths"
)

func TestDiscover_WordCountTemplate(t *testing.T) {
	tmp := t.TempDir()
	// Layout the discovery expects: the BDP distro dir at <tmp>/distro,
	// exercises in sibling Ejercicio_NN folders.
	distro := filepath.Join(tmp, "distro")
	ex01 := filepath.Join(tmp, "Ejercicio_01")
	if err := os.MkdirAll(distro, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ex01, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"mapper.py", "reducer.py", "data.csv"} {
		if err := os.WriteFile(filepath.Join(ex01, f), []byte("placeholder"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Also create a non-matching folder to make sure discovery filters.
	_ = os.MkdirAll(filepath.Join(tmp, "NotAnExercise"), 0o755)

	p := &paths.Paths{
		ScriptDir:  distro,
		Hadoop:     filepath.Join(distro, "hadoop"),
		HadoopConf: filepath.Join(distro, "hadoop", "etc", "hadoop"),
	}
	got := Discover(p)

	if len(got) != 1 {
		t.Fatalf("discover returned %d exercises, want 1", len(got))
	}
	ex := got[0]
	if ex.ID != "ej01" {
		t.Errorf("id = %q, want %q", ex.ID, "ej01")
	}
	if ex.Template != "wordcount" {
		t.Errorf("template = %q, want %q", ex.Template, "wordcount")
	}
	if len(ex.Steps) != 6 {
		t.Errorf("steps = %d, want 6 (safe-mode-wait + 5 work steps)", len(ex.Steps))
	}
	if len(ex.Requires) == 0 {
		t.Error("WordCount exercise must declare its HDFS prerequisites")
	}
}

func TestDiscover_NoTemplate(t *testing.T) {
	tmp := t.TempDir()
	distro := filepath.Join(tmp, "distro")
	ex := filepath.Join(tmp, "Ejercicio_42")
	_ = os.MkdirAll(distro, 0o755)
	_ = os.MkdirAll(ex, 0o755)
	_ = os.WriteFile(filepath.Join(ex, "readme.md"), []byte("hello"), 0o644)

	got := Discover(&paths.Paths{ScriptDir: distro})
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].Template != "raw" {
		t.Errorf("template = %q, want raw (no mapper.py found)", got[0].Template)
	}
}
