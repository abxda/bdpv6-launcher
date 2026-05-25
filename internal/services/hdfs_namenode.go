package services

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/abxda/bdpv6-launcher/internal/health"
	"github.com/abxda/bdpv6-launcher/internal/logsink"
	"github.com/abxda/bdpv6-launcher/internal/paths"
	"github.com/abxda/bdpv6-launcher/internal/processctl"
)

// HDFSNameNode runs the bundled Hadoop NameNode in the foreground (no
// --daemon flag) so we can capture its stdout/stderr into the logsink. The
// legacy macOS launcher used --daemon and tailed a file; doing it inline
// is simpler and gives us live console output.
type HDFSNameNode struct {
	p    *paths.Paths
	port int // web UI port (9870 by default)

	mu        sync.Mutex
	proc      *processctl.Process
	sink      *logsink.Sink
	startedAt time.Time
	exitCode  int
}

func NewHDFSNameNode(p *paths.Paths, port int) *HDFSNameNode {
	if port <= 0 {
		port = 9870
	}
	return &HDFSNameNode{p: p, port: port, sink: logsink.New("hdfs_namenode", p.Logs, 2000), exitCode: -1}
}

func (n *HDFSNameNode) ID() string          { return "hdfs_namenode" }
func (n *HDFSNameNode) Name() string        { return "HDFS NameNode" }
func (n *HDFSNameNode) Port() int           { return n.port }
func (n *HDFSNameNode) Logs() *logsink.Sink { return n.sink }

// RequiredPorts returns BOTH ports the NameNode needs free before Start:
// the web UI (typically 9870, but honors the override) and the RPC port
// (9000, hardcoded in core-site.xml). Surfaced by the App's preflight
// so a zombie on either port aborts before we spawn another JVM that
// would crash with Address already in use.
func (n *HDFSNameNode) RequiredPorts() []int { return []int{n.port, 9000} }

func (n *HDFSNameNode) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.proc != nil && n.proc.Running() {
		return nil
	}
	env := javaEnv(n.p)
	// Hadoop on macOS expects HDFS_*_USER vars; use $USER, fallback "bdp".
	env["HDFS_NAMENODE_USER"] = userOrDefault()
	n.sink.Emit("INFO", "Lanzando HDFS NameNode (foreground)")
	spec := processctl.Spec{
		Command: n.p.HdfsCommand(),
		Args:    []string{"namenode"},
		Env:     env,
		Cwd:     n.p.Hadoop,
		Out:     sinkWriter{s: n.sink},
	}
	proc := processctl.New(spec)
	if err := proc.Start(); err != nil {
		n.sink.Emit("ERROR", "Fallo al lanzar NameNode: "+err.Error())
		return err
	}
	n.proc = proc
	n.startedAt = time.Now()
	n.sink.Emit("INFO", fmt.Sprintf("NameNode arrancado con PID %d", proc.PID()))
	return nil
}

// GracefulPreStop forces HDFS to checkpoint fsimage to disk before we hard-
// kill the process. Without this, the JVM never runs its shutdown hooks and
// you end up with a current/ dir holding edits_inprogress_N but no VERSION
// and no fsimage — Hadoop's definition of corrupted.
//
// The sequence is the same one Hadoop docs recommend for a safe NameNode
// shutdown:
//
//	hdfs dfsadmin -safemode enter      → stop accepting writes
//	hdfs dfsadmin -saveNamespace       → flush fsimage to disk
//	hdfs dfsadmin -safemode leave      → restore writes
//
// After this, the subsequent Stop's taskkill /F /T loses no data because
// fsimage is already on disk. Each subcommand spawns its own JVM (~3-5 s
// startup) so the whole dance is ~10-15 s. Best-effort: if any step fails
// (NameNode not actually responsive, dfsadmin times out, …) we return the
// error but the caller proceeds to Stop anyway.
func (n *HDFSNameNode) GracefulPreStop(ctx context.Context) error {
	n.mu.Lock()
	proc := n.proc
	n.mu.Unlock()
	if proc == nil || !proc.Running() {
		return nil
	}
	n.sink.Emit("INFO", "Forzando checkpoint de fsimage antes de detener…")

	for _, args := range [][]string{
		{"-safemode", "enter"},
		{"-saveNamespace"},
		{"-safemode", "leave"},
	} {
		if err := n.runDfsAdmin(ctx, args...); err != nil {
			// Don't propagate: still want Stop to fire. The corruption
			// detector + auto-cleanup in setup will rescue us on next run.
			n.sink.Emit("WARN", "Paso fallido del checkpoint: "+err.Error())
			return nil
		}
	}
	n.sink.Emit("INFO", "Checkpoint de fsimage guardado en disco.")
	return nil
}

// runDfsAdmin shells out to hdfs.cmd dfsadmin with a per-call 20 s timeout.
// We capture combined output and stream every line into the namenode sink so
// the user can watch the checkpoint progress in the diagnostic console.
func (n *HDFSNameNode) runDfsAdmin(parent context.Context, args ...string) error {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()

	cmdArgs := append([]string{"dfsadmin"}, args...)
	cmd := exec.CommandContext(ctx, n.p.HdfsCommand(), cmdArgs...)
	cmd.Dir = n.p.Hadoop
	cmd.Env = mergeOSEnvFor(javaEnv(n.p), map[string]string{
		"HDFS_NAMENODE_USER": userOrDefault(),
	})
	cmd.SysProcAttr = hideWindowSysAttr()

	out, err := cmd.CombinedOutput()
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			n.sink.Emit("INFO", "[dfsadmin "+strings.Join(args, " ")+"] "+line)
		}
	}
	return err
}

func (n *HDFSNameNode) Stop(ctx context.Context) error {
	n.mu.Lock()
	proc := n.proc
	n.mu.Unlock()
	if proc == nil {
		return nil
	}
	n.sink.Emit("INFO", "Deteniendo HDFS NameNode…")
	err := proc.Stop(15 * time.Second)
	n.mu.Lock()
	n.exitCode = proc.ExitCode()
	n.mu.Unlock()
	if err != nil {
		n.sink.Emit("ERROR", "Stop devolvió error: "+err.Error())
	} else {
		n.sink.Emit("INFO", fmt.Sprintf("NameNode detenido (exit %d)", proc.ExitCode()))
	}
	return err
}

func (n *HDFSNameNode) Status() Status {
	n.mu.Lock()
	proc := n.proc
	since := n.startedAt
	exit := n.exitCode
	n.mu.Unlock()
	st := Status{ID: n.ID(), Name: n.Name(), Port: n.port, ExitCode: exit}
	if proc != nil && proc.Running() {
		st.Running = true
		st.PID = proc.PID()
		st.Since = since
		probe := health.HTTP(fmt.Sprintf("http://127.0.0.1:%d/", n.port), 1500*time.Millisecond)
		st.Healthy = probe.Healthy
		st.Detail = probe.Detail
	} else {
		st.Detail = "detenido"
	}
	return st
}
