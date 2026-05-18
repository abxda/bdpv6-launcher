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

// Kafka wraps the bundled Kafka KRaft single-node cluster. Port refers to
// the broker listener; the controller port is broker+1 by convention.
type Kafka struct {
	p    *paths.Paths
	port int
	heap string

	mu        sync.Mutex
	proc      *processctl.Process
	sink      *logsink.Sink
	startedAt time.Time
	exitCode  int
}

func NewKafka(p *paths.Paths, port int, heap string) *Kafka {
	if port <= 0 {
		port = 9092
	}
	return &Kafka{
		p:        p,
		port:     port,
		heap:     heap,
		sink:     logsink.New("kafka", p.Logs, 2000),
		exitCode: -1,
	}
}

func (k *Kafka) ID() string             { return "kafka" }
func (k *Kafka) Name() string           { return "Kafka" }
func (k *Kafka) Port() int              { return k.port }
func (k *Kafka) Logs() *logsink.Sink    { return k.sink }

func (k *Kafka) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.proc != nil && k.proc.Running() {
		return nil
	}

	env := javaEnv(k.p)
	if k.heap != "" {
		env["KAFKA_HEAP_OPTS"] = fmt.Sprintf("-Xms%s -Xmx%s", k.heap, k.heap)
	}

	k.sink.Emit("INFO", fmt.Sprintf("Lanzando Kafka KRaft en puerto %d", k.port))
	spec := processctl.Spec{
		Command: k.p.KafkaStartCommand(),
		Args:    []string{k.p.KafkaConfig()},
		Env:     env,
		Cwd:     k.p.Kafka,
		Out:     sinkWriter{s: k.sink},
	}
	proc := processctl.New(spec)
	if err := proc.Start(); err != nil {
		k.sink.Emit("ERROR", "Fallo al lanzar Kafka: "+err.Error())
		return err
	}
	k.proc = proc
	k.startedAt = time.Now()
	k.sink.Emit("INFO", fmt.Sprintf("Kafka arrancado con PID %d", proc.PID()))
	return nil
}

func (k *Kafka) Stop(ctx context.Context) error {
	k.mu.Lock()
	proc := k.proc
	k.mu.Unlock()
	if proc == nil {
		return nil
	}
	k.sink.Emit("INFO", "Deteniendo Kafka…")
	err := proc.Stop(10 * time.Second)
	k.mu.Lock()
	k.exitCode = proc.ExitCode()
	k.mu.Unlock()
	if err != nil {
		k.sink.Emit("ERROR", "Stop devolvió error: "+err.Error())
	} else {
		k.sink.Emit("INFO", fmt.Sprintf("Kafka detenido (exit %d)", proc.ExitCode()))
	}
	return err
}

func (k *Kafka) Status() Status {
	k.mu.Lock()
	proc := k.proc
	since := k.startedAt
	exit := k.exitCode
	k.mu.Unlock()
	st := Status{ID: k.ID(), Name: k.Name(), Port: k.port, ExitCode: exit}
	if proc != nil && proc.Running() {
		st.Running = true
		st.PID = proc.PID()
		st.Since = since
		probe := health.TCP(fmt.Sprintf("127.0.0.1:%d", k.port), 500*time.Millisecond)
		st.Healthy = probe.Healthy
		st.Detail = probe.Detail
	} else {
		st.Detail = "detenido"
	}
	return st
}
