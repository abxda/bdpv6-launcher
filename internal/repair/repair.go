// Package repair exposes the destructive maintenance actions that students
// used to perform by running cleanup.bat / repair_jupyter.sh / setup_first_run.bat
// by hand. Each action streams its output through a logsink.Sink (id "repair")
// so the UI shows progress live, and every action expects the caller to
// have already obtained explicit user confirmation.
package repair

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/abxda/bdpv6-launcher/internal/logsink"
	"github.com/abxda/bdpv6-launcher/internal/paths"
	"github.com/abxda/bdpv6-launcher/internal/setup"
	"github.com/abxda/bdpv6-launcher/internal/state"
)

// Action enumerates the repair operations the UI can request.
type Action string

const (
	ActionRepairJupyter  Action = "repair_jupyter"
	ActionClean          Action = "clean"
	ActionReformatHDFS   Action = "reformat_hdfs"
	ActionReformatKafka  Action = "reformat_kafka"
)

// Repairer dispatches an Action against the live distribution.
type Repairer struct {
	p     *paths.Paths
	state *state.Store
	sink  *logsink.Sink
	setup *setup.Orchestrator
}

func New(p *paths.Paths, st *state.Store, sink *logsink.Sink, s *setup.Orchestrator) *Repairer {
	return &Repairer{p: p, state: st, sink: sink, setup: s}
}

// Run dispatches to the matching action, streaming output via the sink.
// Errors are returned to the caller and also Emit()-ed for the UI.
func (r *Repairer) Run(ctx context.Context, a Action) error {
	r.sink.Emit("INFO", strings.Repeat("=", 60))
	r.sink.Emit("INFO", "Ejecutando acción: "+string(a))

	var err error
	switch a {
	case ActionRepairJupyter:
		err = r.repairJupyter(ctx)
	case ActionClean:
		err = r.cleanData(ctx)
	case ActionReformatHDFS:
		// Delegate to the same code path the wizard uses, but via the
		// setup orchestrator so its status flips back to "done".
		err = r.setup.RunStep(ctx, "hdfs")
	case ActionReformatKafka:
		err = r.setup.RunStep(ctx, "kafka")
	default:
		err = fmt.Errorf("acción desconocida: %s", a)
	}

	if err != nil {
		r.sink.Emit("ERROR", "Acción fallida: "+err.Error())
	} else {
		r.sink.Emit("INFO", "Acción completada: "+string(a))
	}
	return err
}

// --- repair_jupyter: uninstall + reinstall the Jupyter stack -------------

func (r *Repairer) repairJupyter(ctx context.Context) error {
	pkgs := []string{"jupyter_core", "jupyter_client", "jupyter_server", "notebook", "jupyterlab"}

	if err := r.runPipStep(ctx, "Desinstalando paquetes Jupyter", append([]string{"-m", "pip", "uninstall", "-y"}, pkgs...)); err != nil {
		return err
	}
	if err := r.runPipStep(ctx, "Reinstalando paquetes Jupyter", append([]string{"-m", "pip", "install", "--upgrade"}, pkgs...)); err != nil {
		return err
	}
	r.sink.Emit("INFO", "Reparación de Jupyter completada.")
	return nil
}

func (r *Repairer) runPipStep(ctx context.Context, label string, args []string) error {
	r.sink.Emit("INFO", label+" — esto puede tardar 1-3 minutos…")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.p.PythonBinary(), args...)
	cmd.Dir = r.p.ScriptDir
	cmd.SysProcAttr = hideWindowAttr()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	go streamLines(stdout, func(s string) { r.sink.Emit("INFO", s) })
	go streamLines(stderr, func(s string) { r.sink.Emit("INFO", s) })
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s falló: %w", label, err)
	}
	return nil
}

// --- clean: wipe data/ + logs/ + clear setupCompleted ---------------------

// cleanData rm-rfs the data and logs directories, then recreates them empty.
// Also resets state.SetupCompleted so the wizard prompts again on next
// launch (because HDFS will need re-formatting). Mirrors cleanup.bat /
// cleanup.sh from the legacy distros.
func (r *Repairer) cleanData(ctx context.Context) error {
	for _, dir := range []string{r.p.Data, r.p.Logs} {
		if dir == "" {
			continue
		}
		r.sink.Emit("INFO", "Borrando "+dir)
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("rm -rf %s: %w", dir, err)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	// Clean __pycache__ recursively under python/ so a broken interpreter
	// can be reset along with everything else. Best-effort.
	if r.p.Python != "" {
		count := 0
		_ = filepath.Walk(r.p.Python, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || !info.IsDir() {
				return nil
			}
			if info.Name() == "__pycache__" {
				_ = os.RemoveAll(path)
				count++
				return filepath.SkipDir
			}
			return nil
		})
		r.sink.Emit("INFO", fmt.Sprintf("Eliminados %d directorios __pycache__", count))
	}

	// Reset the SetupCompleted flag so the wizard reappears.
	if err := r.state.Update(func(s *state.State) {
		s.SetupCompleted = false
	}); err != nil {
		r.sink.Emit("WARN", "No pude resetear state.SetupCompleted: "+err.Error())
	}
	r.sink.Emit("INFO", "Limpieza completada. Necesitarás re-ejecutar la configuración inicial.")
	return nil
}

// --- helpers -------------------------------------------------------------

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

// ErrUnknownAction is returned when an unsupported Action is requested.
var ErrUnknownAction = errors.New("acción desconocida")
