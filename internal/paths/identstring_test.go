package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Reproduce la situación real: hadoop-env.cmd con la línea comentada (@rem) y la
// línea ACTIVA que asigna %USERNAME%, en CRLF. El fix debe tocar solo la activa,
// dejar la @rem intacta, preservar CRLF y ser idempotente.
func TestEnsureHadoopIdentString(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "hadoop", "etc", "hadoop")
	if err := os.MkdirAll(conf, 0o755); err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(conf, "hadoop-env.cmd")
	orig := "@rem A string representing this instance of hadoop. %USERNAME% by default.\r\n" +
		"@rem set HADOOP_IDENT_STRING=%USERNAME%\r\n" +
		"set HADOOP_IDENT_STRING=%USERNAME%\r\n" +
		"set HADOOP_PID_DIR=%HADOOP_PID_DIR%\r\n"
	if err := os.WriteFile(env, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &Paths{HadoopConf: conf}

	changed, err := p.EnsureHadoopIdentString()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !changed {
		t.Fatal("esperaba changed=true en la primera pasada")
	}
	out, _ := os.ReadFile(env)
	s := string(out)
	if !strings.Contains(s, "set HADOOP_IDENT_STRING=bdp\r\n") {
		t.Errorf("no quedó la línea fija sin espacios:\n%s", s)
	}
	if !strings.Contains(s, "@rem set HADOOP_IDENT_STRING=%USERNAME%\r\n") {
		t.Errorf("se tocó la línea comentada (@rem), no debía:\n%s", s)
	}
	if !strings.Contains(s, "set HADOOP_PID_DIR=%HADOOP_PID_DIR%\r\n") {
		t.Errorf("se dañó otra línea:\n%s", s)
	}
	// Ancla a inicio de línea: la activa es "\nset ...", la comentada es "\n@rem set ...".
	if strings.Contains(s, "\nset HADOOP_IDENT_STRING=%USERNAME%") {
		t.Errorf("la línea activa con %%USERNAME%% sigue presente:\n%s", s)
	}

	// Idempotente: segunda pasada no cambia nada.
	changed2, err := p.EnsureHadoopIdentString()
	if err != nil {
		t.Fatalf("err 2: %v", err)
	}
	if changed2 {
		t.Error("esperaba changed=false en la segunda pasada (idempotente)")
	}
}

// Si no hay hadoop-env.cmd (no es Windows), es un no-op silencioso.
func TestEnsureHadoopIdentString_NoFile(t *testing.T) {
	p := &Paths{HadoopConf: t.TempDir()}
	changed, err := p.EnsureHadoopIdentString()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if changed {
		t.Error("sin archivo no debe cambiar nada")
	}
}
