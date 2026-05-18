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

// Elasticsearch wraps the bundled ES distribution. It is the simplest of the
// BDP services (single binary, no pre-format, single port), so it is
// implemented first and exercises the full Service contract end-to-end.
type Elasticsearch struct {
	p        *paths.Paths
	port     int
	heap     string

	mu       sync.Mutex
	proc     *processctl.Process
	sink     *logsink.Sink
	startedAt time.Time
	exitCode int
}

// NewElasticsearch constructs an idle wrapper. The Logs() Sink is created
// eagerly so subscribers can attach before Start.
func NewElasticsearch(p *paths.Paths, port int, heap string) *Elasticsearch {
	if port <= 0 {
		port = 9200
	}
	return &Elasticsearch{
		p:        p,
		port:     port,
		heap:     heap,
		sink:     logsink.New("elasticsearch", p.Logs, 2000),
		exitCode: -1,
	}
}

func (e *Elasticsearch) ID() string   { return "elasticsearch" }
func (e *Elasticsearch) Name() string { return "Elasticsearch" }
func (e *Elasticsearch) Port() int    { return e.port }
func (e *Elasticsearch) Logs() *logsink.Sink { return e.sink }

func (e *Elasticsearch) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.proc != nil && e.proc.Running() {
		return nil
	}

	env := map[string]string{
		"JAVA_HOME":      e.p.CommonJDK,
		"ES_JAVA_OPTS":   fmt.Sprintf("-Xms%s -Xmx%s", e.heapOrDefault(), e.heapOrDefault()),
	}

	args := []string{}
	// If the user changed the port, override via -E flag; otherwise leave the
	// elasticsearch.yml default in place.
	if e.port != 9200 {
		args = append(args, "-E", fmt.Sprintf("http.port=%d", e.port))
	}

	e.sink.Emit("INFO", fmt.Sprintf("Lanzando Elasticsearch en puerto %d (heap %s)", e.port, e.heapOrDefault()))

	spec := processctl.Spec{
		Command: e.p.ElasticBinary(),
		Args:    args,
		Env:     env,
		Cwd:     e.p.Elastic,
		Out:     elasticWriter{e.sink},
	}
	proc := processctl.New(spec)
	if err := proc.Start(); err != nil {
		e.sink.Emit("ERROR", "Fallo al lanzar Elasticsearch: "+err.Error())
		return err
	}
	e.proc = proc
	e.startedAt = time.Now()
	e.sink.Emit("INFO", fmt.Sprintf("Elasticsearch arrancado con PID %d", proc.PID()))
	return nil
}

func (e *Elasticsearch) Stop(ctx context.Context) error {
	e.mu.Lock()
	proc := e.proc
	e.mu.Unlock()
	if proc == nil {
		return nil
	}
	e.sink.Emit("INFO", "Deteniendo Elasticsearch…")
	err := proc.Stop(10 * time.Second)
	e.mu.Lock()
	e.exitCode = proc.ExitCode()
	e.mu.Unlock()
	if err != nil {
		e.sink.Emit("ERROR", "Stop devolvió error: "+err.Error())
	} else {
		e.sink.Emit("INFO", fmt.Sprintf("Elasticsearch detenido (exit %d)", proc.ExitCode()))
	}
	return err
}

func (e *Elasticsearch) Status() Status {
	e.mu.Lock()
	proc := e.proc
	since := e.startedAt
	exit := e.exitCode
	e.mu.Unlock()

	st := Status{
		ID: e.ID(), Name: e.Name(), Port: e.port, ExitCode: exit,
	}
	if proc != nil && proc.Running() {
		st.Running = true
		st.PID = proc.PID()
		st.Since = since
		probe := health.HTTP(fmt.Sprintf("http://127.0.0.1:%d/_cluster/health?timeout=1s", e.port), 1500*time.Millisecond)
		st.Healthy = probe.Healthy
		st.Detail = probe.Detail
	} else {
		st.Running = false
		st.Detail = "detenido"
	}
	return st
}

func (e *Elasticsearch) heapOrDefault() string {
	if e.heap == "" {
		return "1g"
	}
	return e.heap
}

// elasticWriter adapts logsink.Sink to io.Writer for the processctl Out hook.
// It splits on '\n' so the Sink stores one Line per terminal line even when
// the child writes long bursts.
type elasticWriter struct{ s *logsink.Sink }

func (w elasticWriter) Write(p []byte) (int, error) {
	// We use Attach() for clean line-buffering, so route raw output through a
	// pair of pipes. The simpler path: split here on '\n' and Emit each.
	start := 0
	for i, b := range p {
		if b == '\n' {
			line := string(p[start:i])
			w.s.Emit("INFO", trimCarriage(line))
			start = i + 1
		}
	}
	if start < len(p) {
		w.s.Emit("INFO", trimCarriage(string(p[start:])))
	}
	return len(p), nil
}

func trimCarriage(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\r' {
		return s[:len(s)-1]
	}
	return s
}
