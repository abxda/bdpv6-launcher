package services

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/abxda/bdpv6-launcher/internal/health"
	"github.com/abxda/bdpv6-launcher/internal/logsink"
	"github.com/abxda/bdpv6-launcher/internal/paths"
	"github.com/abxda/bdpv6-launcher/internal/processctl"
)

type HDFSDataNode struct {
	p    *paths.Paths
	port int // web UI port (9864 by default)

	mu        sync.Mutex
	proc      *processctl.Process
	sink      *logsink.Sink
	startedAt time.Time
	exitCode  int
}

func NewHDFSDataNode(p *paths.Paths, port int) *HDFSDataNode {
	if port <= 0 {
		port = 9864
	}
	return &HDFSDataNode{p: p, port: port, sink: logsink.New("hdfs_datanode", p.Logs, 2000), exitCode: -1}
}

func (d *HDFSDataNode) ID() string          { return "hdfs_datanode" }
func (d *HDFSDataNode) Name() string        { return "HDFS DataNode" }
func (d *HDFSDataNode) Port() int           { return d.port }
func (d *HDFSDataNode) Logs() *logsink.Sink { return d.sink }

func (d *HDFSDataNode) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.proc != nil && d.proc.Running() {
		return nil
	}
	env := javaEnv(d.p)
	env["HDFS_DATANODE_USER"] = userOrDefault()
	d.sink.Emit("INFO", "Lanzando HDFS DataNode")
	spec := processctl.Spec{
		Command: d.p.HdfsCommand(),
		Args:    []string{"datanode"},
		Env:     env,
		Cwd:     d.p.Hadoop,
		Out:     sinkWriter{s: d.sink},
	}
	proc := processctl.New(spec)
	if err := proc.Start(); err != nil {
		d.sink.Emit("ERROR", "Fallo al lanzar DataNode: "+err.Error())
		return err
	}
	d.proc = proc
	d.startedAt = time.Now()
	d.sink.Emit("INFO", fmt.Sprintf("DataNode arrancado con PID %d", proc.PID()))
	return nil
}

func (d *HDFSDataNode) Stop(ctx context.Context) error {
	d.mu.Lock()
	proc := d.proc
	d.mu.Unlock()
	if proc == nil {
		return nil
	}
	d.sink.Emit("INFO", "Deteniendo HDFS DataNode…")
	err := proc.Stop(15 * time.Second)
	d.mu.Lock()
	d.exitCode = proc.ExitCode()
	d.mu.Unlock()
	if err != nil {
		d.sink.Emit("ERROR", "Stop devolvió error: "+err.Error())
	} else {
		d.sink.Emit("INFO", fmt.Sprintf("DataNode detenido (exit %d)", proc.ExitCode()))
	}
	return err
}

func (d *HDFSDataNode) Status() Status {
	d.mu.Lock()
	proc := d.proc
	since := d.startedAt
	exit := d.exitCode
	d.mu.Unlock()
	st := Status{ID: d.ID(), Name: d.Name(), Port: d.port, ExitCode: exit}
	if proc != nil && proc.Running() {
		st.Running = true
		st.PID = proc.PID()
		st.Since = since
		probe := health.HTTP(fmt.Sprintf("http://127.0.0.1:%d/", d.port), 1500*time.Millisecond)
		st.Healthy = probe.Healthy
		st.Detail = probe.Detail
	} else {
		st.Detail = "detenido"
	}
	return st
}

// userOrDefault returns $USER (Unix) / $USERNAME (Windows) with a fallback
// to "bdp" if the env var is empty. Used for Hadoop's HDFS_*_USER vars on
// macOS — os.getlogin() in the legacy launcher could panic in headless
// sessions, so we read the env directly here.
func userOrDefault() string {
	for _, k := range []string{"USER", "USERNAME"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "bdp"
}
