package services

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/abxda/bdpv6-launcher/internal/health"
	"github.com/abxda/bdpv6-launcher/internal/logsink"
	"github.com/abxda/bdpv6-launcher/internal/paths"
	"github.com/abxda/bdpv6-launcher/internal/processctl"
)

// Kafka wraps the bundled Kafka KRaft single-node cluster. Port refers to
// the broker listener; the controller port is broker+1 by convention.
//
// IMPORTANT: we bypass the bundled kafka-server-start.bat wrapper entirely
// and invoke `java` directly. The .bat file builds a CLASSPATH by listing
// every jar in kafka_kraft/libs/* and pastes it onto the command line via
// `java -cp "<6900 chars>" …`. On Windows that overflows the 8191-char
// cmd.exe limit and the bat dies with "La sintaxis del comando no es
// correcta." before Java even starts. Calling java with the wildcard
// classpath `libs/*` keeps the command line short and works on both
// Windows and macOS without depending on the OS-specific wrappers.
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

// RequiredPorts returns the broker listener (default 9092) and the
// controller listener (9093, defined in server.properties).
func (k *Kafka) RequiredPorts() []int { return []int{k.port, 9093} }

func (k *Kafka) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.proc != nil && k.proc.Running() {
		return nil
	}

	heap := k.heap
	if heap == "" {
		heap = "512m"
	}

	classpath := KafkaWildcardClasspath(k.p)
	log4j := filepath.Join(k.p.Kafka, "config", "log4j.properties")

	args := []string{
		"-Xms" + heap, "-Xmx" + heap,
		"-server",
		"-XX:+UseG1GC",
		"-XX:MaxGCPauseMillis=20",
		"-Dlog4j.configuration=file:" + filepath.ToSlash(log4j),
		"-Dkafka.logs.dir=" + filepath.ToSlash(filepath.Join(k.p.Logs, "kafka")),
		"-classpath", classpath,
		"kafka.Kafka",
		k.p.KafkaConfig(),
	}

	env := javaEnv(k.p)

	k.sink.Emit("INFO", fmt.Sprintf("Lanzando Kafka KRaft en puerto %d (heap %s)", k.port, heap))
	spec := processctl.Spec{
		Command: k.p.JavaBinary(),
		Args:    args,
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

// KafkaWildcardClasspath returns "<kafka>/libs/*" which Java expands at
// startup to every jar in the folder. Exported so the setup package can
// reuse it for kafka-storage format without duplicating logic.
func KafkaWildcardClasspath(p *paths.Paths) string {
	return filepath.Join(p.Kafka, "libs", "*")
}
