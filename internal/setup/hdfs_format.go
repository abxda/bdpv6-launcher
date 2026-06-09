package setup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// runHDFS executes `hdfs namenode -format -force -nonInteractive`. The
// command can run for a few seconds; we stream every output line into the
// sink so students see progress instead of staring at a frozen modal.
//
// Before formatting we recover from common broken states left behind by
// previous force-kills: a stale in_use.lock (which would make HDFS refuse
// to start later) and a corrupted current/ dir (existing edits but no
// VERSION, which Hadoop sometimes treats as "partially formatted" and
// also refuses). The user's namespace metadata is already gone in that
// case; the only path forward is a clean reformat.
func (o *Orchestrator) runHDFS(ctx context.Context) error {
	// Ensure the data dirs exist — Hadoop will create them but fails noisily
	// on Windows if the parent is missing.
	if err := os.MkdirAll(o.p.Data, 0o755); err != nil {
		return fmt.Errorf("mkdir data: %w", err)
	}

	// Best-effort recovery from previously corrupted state.
	o.cleanupBeforeFormat()

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	env := os.Environ()
	env = appendEnv(env, "JAVA_HOME", o.p.CommonJDK)
	env = appendEnv(env, "HADOOP_HOME", o.p.Hadoop)
	env = appendEnv(env, "HADOOP_CONF_DIR", o.p.HadoopConf)
	env = appendEnv(env, "HDFS_NAMENODE_USER", userOrDefault())

	cmd := exec.CommandContext(ctx, o.p.HdfsCommand(), "namenode", "-format", "-force", "-nonInteractive")
	cmd.Env = env
	cmd.Dir = o.p.Hadoop
	cmd.SysProcAttr = hideWindowAttr()

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("hdfs format start: %w", err)
	}

	// En Windows el .cmd de Hadoop a veces sale con código 0 AUNQUE el java
	// interno falle (p.ej. -Dhadoop.id.str con ESPACIOS -> "Could not find or
	// load main class PC"), por eso el exit code no basta. Detectamos el fallo de
	// dos formas: (1) escaneando la salida por la firma del error de Java, y
	// (2) verificando el RESULTADO (que el formateo haya creado current/VERSION).
	var mu sync.Mutex
	fatal := ""
	detect := func(s string) {
		low := strings.ToLower(s)
		if strings.Contains(low, "could not find or load main class") ||
			strings.Contains(low, "classnotfoundexception") ||
			strings.Contains(low, "no se ha encontrado o cargado la clase") {
			mu.Lock()
			if fatal == "" {
				fatal = strings.TrimSpace(s)
			}
			mu.Unlock()
		}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); streamLines(stdout, func(s string) { o.sink.Emit("INFO", s); detect(s) }) }()
	go func() { defer wg.Done(); streamLines(stderr, func(s string) { o.sink.Emit("INFO", s); detect(s) }) }()

	werr := cmd.Wait()
	wg.Wait() // asegura que leímos TODA la salida antes de juzgar el resultado

	if werr != nil {
		return fmt.Errorf("hdfs format wait: %w", werr)
	}
	mu.Lock()
	f := fatal
	mu.Unlock()
	if f != "" || !o.p.NamenodeFormatted() {
		msg := "el formateo de HDFS FALLÓ (el NameNode no quedó formateado)"
		if f != "" {
			msg += " — Java: " + f
		}
		msg += ". Causa típica en Windows: ESPACIOS en el nombre de usuario o en la ruta del " +
			"laboratorio. Cierra y reabre el launcher (se auto-actualiza con el arreglo) y usa una " +
			"ruta SIN espacios, por ejemplo C:\\BDP."
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// cleanupBeforeFormat wipes leftovers from a previous bad shutdown so the
// upcoming `hdfs namenode -format` succeeds cleanly. Three things:
//
//  1. in_use.lock at the namenode root — held by the JVM while running;
//     left behind when we force-kill. Hadoop won't start while it exists.
//  2. current/ dir without a VERSION file — means a previous JVM was
//     killed mid-format, or started up, opened edits, and was killed
//     before checkpointing fsimage. Either way the on-disk namespace is
//     unrecoverable; we delete it so format starts from clean state.
//  3. The datanode side gets the same treatment for symmetry. A datanode
//     keyed to a deleted namenode would fail with cluster-id mismatch on
//     next start anyway, so wiping it now is the correct move.
//
// Every step is best-effort: on failure (file locked by a live process,
// permission denied, …) we log a warning and continue; the format step
// itself will then surface a clearer error.
func (o *Orchestrator) cleanupBeforeFormat() {
	nnRoot := filepath.Join(o.p.Data, "hdfs", "namenode")
	nnCurr := filepath.Join(nnRoot, "current")
	dnRoot := filepath.Join(o.p.Data, "hdfs", "datanode")

	// 1. Always remove a stale in_use.lock if any.
	for _, lock := range []string{
		filepath.Join(nnRoot, "in_use.lock"),
		filepath.Join(dnRoot, "in_use.lock"),
	} {
		if err := os.Remove(lock); err == nil {
			o.sink.Emit("WARN", "Lock huérfano removido: "+lock)
		}
	}

	// 2. If namenode/current exists but VERSION is missing → corrupted.
	if _, err := os.Stat(nnCurr); err == nil {
		if _, vErr := os.Stat(filepath.Join(nnCurr, "VERSION")); os.IsNotExist(vErr) {
			o.sink.Emit("WARN", "Estado corrupto detectado en "+nnCurr+" (sin VERSION). Limpiando para reformatear…")
			if err := os.RemoveAll(nnCurr); err != nil {
				o.sink.Emit("ERROR", "No pude limpiar "+nnCurr+": "+err.Error())
			}
		}
	}

	// 3. Wipe the datanode side too — a datanode that survives the format
	// of its namenode would fail to register with the new clusterID.
	if _, err := os.Stat(dnRoot); err == nil {
		o.sink.Emit("INFO", "Limpiando "+dnRoot+" para alinearlo con el nuevo namenode…")
		if err := os.RemoveAll(dnRoot); err != nil {
			o.sink.Emit("WARN", "No pude limpiar "+dnRoot+": "+err.Error())
		}
	}
}

func streamLines(r io.Reader, emit func(string)) {
	if r == nil {
		return
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		emit(strings.TrimRight(sc.Text(), "\r"))
	}
}

func appendEnv(env []string, k, v string) []string {
	prefix := k + "="
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			out = append(out, prefix+v)
			replaced = true
		} else {
			out = append(out, kv)
		}
	}
	if !replaced {
		out = append(out, prefix+v)
	}
	return out
}

// userOrDefault duplicates the helper in services package to avoid a cyclic
// import.
func userOrDefault() string {
	for _, k := range []string{"USER", "USERNAME"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "bdp"
}
