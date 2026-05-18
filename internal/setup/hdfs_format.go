package setup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// runHDFS executes `hdfs namenode -format -force -nonInteractive`. The
// command can run for a few seconds; we stream every output line into the
// sink so students see progress instead of staring at a frozen modal.
func (o *Orchestrator) runHDFS(ctx context.Context) error {
	// Ensure the data dirs exist — Hadoop will create them but fails noisily
	// on Windows if the parent is missing.
	if err := os.MkdirAll(o.p.Data, 0o755); err != nil {
		return fmt.Errorf("mkdir data: %w", err)
	}

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
	go streamLines(stdout, func(s string) { o.sink.Emit("INFO", s) })
	go streamLines(stderr, func(s string) { o.sink.Emit("INFO", s) })

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("hdfs format wait: %w", err)
	}
	return nil
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
