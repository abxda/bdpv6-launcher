// Package setup runs the first-launch wizard that prepares a fresh BDP
// distribution for use: verify the bundled JDK, generate Hadoop config
// files that point at the current SCRIPT_DIR, format HDFS, and format the
// Kafka KRaft metadata log.
//
// Each step streams its output through a logsink.Sink so the wizard UI can
// render the same live console used by the diagnostic tabs. Steps are
// idempotent — re-running them after a partial failure is safe.
package setup

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/abxda/bdpv6-launcher/internal/logsink"
	"github.com/abxda/bdpv6-launcher/internal/paths"
	"github.com/abxda/bdpv6-launcher/internal/state"
)

// StepStatus is the state of a single wizard step.
type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepRunning StepStatus = "running"
	StepDone    StepStatus = "done"
	StepFailed  StepStatus = "failed"
)

type Step struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Status      StepStatus `json:"status"`
	ErrorMsg    string     `json:"errorMsg,omitempty"`
}

// Orchestrator owns the wizard state and exposes a single RunStep method.
// A successful run of all four steps automatically flips state.SetupCompleted.
type Orchestrator struct {
	p     *paths.Paths
	state *state.Store
	sink  *logsink.Sink

	mu    sync.Mutex
	steps map[string]*Step
	order []string
}

func New(p *paths.Paths, st *state.Store, sink *logsink.Sink) *Orchestrator {
	o := &Orchestrator{p: p, state: st, sink: sink, steps: map[string]*Step{}}
	o.add("jdk",   "Verificar JDK",           "Ejecuta common_jdk/bin/java -version para confirmar que el JDK empaquetado es alcanzable.")
	o.add("xml",   "Generar configs de Hadoop","Escribe core-site.xml y hdfs-site.xml con rutas file:/// absolutas al SCRIPT_DIR actual.")
	o.add("hdfs",  "Formatear HDFS",          "Ejecuta hdfs namenode -format -force -nonInteractive sobre el directorio data/hdfs/.")
	o.add("kafka", "Formatear Kafka",         "Genera un UUID nuevo y ejecuta kafka-storage format sobre server.properties.")
	return o
}

func (o *Orchestrator) add(id, name, desc string) {
	o.steps[id] = &Step{ID: id, Name: name, Description: desc, Status: StepPending}
	o.order = append(o.order, id)
}

// Steps returns a snapshot in declaration order for the UI.
func (o *Orchestrator) Steps() []Step {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]Step, 0, len(o.order))
	for _, id := range o.order {
		if s := o.steps[id]; s != nil {
			out = append(out, *s)
		}
	}
	return out
}

// RunStep executes one step and updates its status. Errors are surfaced both
// in the returned error and in the Step.ErrorMsg field.
func (o *Orchestrator) RunStep(ctx context.Context, id string) error {
	o.mu.Lock()
	step, ok := o.steps[id]
	if !ok {
		o.mu.Unlock()
		return fmt.Errorf("paso desconocido: %s", id)
	}
	step.Status = StepRunning
	step.ErrorMsg = ""
	o.mu.Unlock()

	o.sink.Emit("INFO", strings.Repeat("=", 60))
	o.sink.Emit("INFO", "Ejecutando paso: "+step.Name)

	var err error
	switch id {
	case "jdk":
		err = o.runJDK(ctx)
	case "xml":
		err = o.runXML(ctx)
	case "hdfs":
		err = o.runHDFS(ctx)
	case "kafka":
		err = o.runKafka(ctx)
	default:
		err = errors.New("paso no implementado")
	}

	o.mu.Lock()
	if err != nil {
		step.Status = StepFailed
		step.ErrorMsg = err.Error()
		o.sink.Emit("ERROR", "Paso fallido: "+err.Error())
	} else {
		step.Status = StepDone
		o.sink.Emit("INFO", "Paso completado: "+step.Name)
	}
	allDone := o.allDoneLocked()
	o.mu.Unlock()

	if allDone {
		if mErr := o.state.MarkSetupCompleted(); mErr != nil {
			o.sink.Emit("WARN", "No pude guardar setupCompleted: "+mErr.Error())
		} else {
			o.sink.Emit("INFO", "Configuración inicial completada. ¡Listo para iniciar servicios!")
		}
	}
	return err
}

// RunAll runs every pending step in order, stopping on the first failure.
func (o *Orchestrator) RunAll(ctx context.Context) error {
	for _, id := range o.order {
		if err := o.RunStep(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (o *Orchestrator) allDoneLocked() bool {
	for _, s := range o.steps {
		if s.Status != StepDone {
			return false
		}
	}
	return true
}

// --- step implementations ---------------------------------------------------

func (o *Orchestrator) runJDK(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, o.p.JavaBinary(), "-version")
	cmd.SysProcAttr = hideWindowAttr()
	out, err := cmd.CombinedOutput()
	// java -version goes to stderr; CombinedOutput merges both for us.
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			o.sink.Emit("INFO", line)
		}
	}
	if err != nil {
		return fmt.Errorf("java -version falló: %w", err)
	}
	return nil
}

func (o *Orchestrator) runXML(ctx context.Context) error {
	if err := WriteHadoopXML(o.p); err != nil {
		return err
	}
	o.sink.Emit("INFO", "core-site.xml escrito en "+o.p.CoreSiteXML())
	o.sink.Emit("INFO", "hdfs-site.xml escrito en "+o.p.HdfsSiteXML())
	return nil
}
