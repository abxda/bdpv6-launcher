package exercises

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/abxda/bdpv6-launcher/internal/logsink"
	"github.com/abxda/bdpv6-launcher/internal/paths"
)

// Runner owns the per-exercise execution state. One Runner per Exercise.
// Sink streams every line of every step's output so the UI can show a
// live console. RunStep / RunAll are mutually exclusive (mu).
type Runner struct {
	ex   Exercise
	p    *paths.Paths
	sink *logsink.Sink

	mu      sync.Mutex
	running bool
}

func NewRunner(ex Exercise, p *paths.Paths, sink *logsink.Sink) *Runner {
	return &Runner{ex: ex, p: p, sink: sink}
}

// RunStep executes one step (0-indexed) and returns when the process
// exits or the context is cancelled.
func (r *Runner) RunStep(ctx context.Context, idx int) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return errors.New("ya hay un paso ejecutándose; espera a que termine")
	}
	r.running = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()

	if idx < 0 || idx >= len(r.ex.Steps) {
		return fmt.Errorf("paso fuera de rango: %d", idx)
	}
	return r.execStep(ctx, r.ex.Steps[idx], idx+1, len(r.ex.Steps))
}

// RunAll executes every step in sequence, stopping on the first failure.
func (r *Runner) RunAll(ctx context.Context) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return errors.New("ya hay un paso ejecutándose; espera a que termine")
	}
	r.running = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()

	total := len(r.ex.Steps)
	for i, s := range r.ex.Steps {
		if err := r.execStep(ctx, s, i+1, total); err != nil {
			r.sink.Emit("ERROR", fmt.Sprintf("Paso %d falló: %v. Detengo la secuencia.", i+1, err))
			return err
		}
	}
	r.sink.Emit("INFO", "✓ Ejercicio completado.")
	return nil
}

func (r *Runner) execStep(ctx context.Context, s Step, num, total int) error {
	r.sink.Emit("INFO", strings.Repeat("─", 60))
	r.sink.Emit("INFO", fmt.Sprintf("[%d/%d] %s", num, total, s.Title))
	if s.Notes != "" {
		r.sink.Emit("INFO", "  ↳ "+s.Notes)
	}
	if s.PrintAs != "" {
		for _, line := range strings.Split(s.PrintAs, "\n") {
			r.sink.Emit("INFO", "$ "+line)
		}
	} else {
		r.sink.Emit("INFO", "$ "+s.Cmd+" "+strings.Join(s.Args, " "))
	}

	if s.Shell && s.Cmd == "" {
		return errors.New("este paso requiere bash y no encontré bash en la distro (esperado en bash/usr/bin/bash.exe)")
	}

	stepCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(stepCtx, s.Cmd, s.Args...)
	cmd.Dir = r.ex.Path
	cmd.Env = r.buildEnv()

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("no pude lanzar el paso: %w", err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); streamLines(stdout, r.sink, "INFO") }()
	go func() { defer wg.Done(); streamLines(stderr, r.sink, "INFO") }()
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

// buildEnv returns the environment the steps inherit. We prepend the
// embedded JDK + Python + Hadoop bin dirs so `hadoop`, `hdfs`, `python`
// and `java` resolve from the BDP distribution regardless of what the
// student has installed on their machine.
func (r *Runner) buildEnv() []string {
	overrides := map[string]string{
		"JAVA_HOME":       r.p.CommonJDK,
		"HADOOP_HOME":     r.p.Hadoop,
		"HADOOP_CONF_DIR": r.p.HadoopConf,
	}
	pathDirs := []string{
		filepath.Join(r.p.CommonJDK, "bin"),
		filepath.Join(r.p.Hadoop, "bin"),
		r.p.Python,
		filepath.Join(r.p.Python, "Scripts"),
	}
	overrides["PATH"] = mergePath(pathDirs)
	return mergeOSEnv(overrides)
}

func streamLines(r io.Reader, sink *logsink.Sink, level string) {
	if r == nil {
		return
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		text := strings.TrimRight(sc.Text(), "\r")
		sink.Emit(level, text)
	}
}
