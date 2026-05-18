package services

import (
	"context"
	"fmt"
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
