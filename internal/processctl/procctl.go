// Package processctl is the cross-platform child-process supervisor used by
// the launcher to start, observe, and stop the Java/Python services bundled
// with the BDP distribution.
//
// Goals (which the legacy PyQt5 launcher missed):
//   - Capture merged stdout/stderr into an io.Writer so the GUI can show
//     diagnostic output live (the legacy launcher dropped it into a separate
//     console window on Windows and a flat file on macOS).
//   - Stop child processes cleanly, including any Java grandchildren spawned
//     by a .bat/.cmd/.sh wrapper. Falls back to a hard kill after a grace
//     period rather than leaving zombies.
//   - Be safe to call Stop() concurrently with Wait() — Stop() returns when
//     either the process exits or the force-kill window elapses.
//
// Platform-specific behaviour (SysProcAttr, graceful-stop signal, hard-kill
// command) lives in procctl_windows.go and procctl_darwin.go.
package processctl

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Spec describes a process to run. Empty fields take sensible defaults.
type Spec struct {
	Command string            // absolute path to the executable / .bat / .sh
	Args    []string          // command-line arguments (already quoted/escaped)
	Env     map[string]string // overrides on top of the current process env
	Cwd     string            // working directory; defaults to the current
	Out     io.Writer         // where merged stdout+stderr goes (e.g. a logsink)
}

// Process is a managed child. Construct with New(); call Start() once, then
// Stop() to terminate. Running()/PID()/ExitCode() are safe at any time.
type Process struct {
	spec Spec

	mu       sync.RWMutex
	cmd      *exec.Cmd
	started  time.Time
	finished time.Time
	exitCode int
	exitErr  error
	done     chan struct{}
}

// New creates an idle Process. No work happens until Start() is called.
func New(spec Spec) *Process {
	return &Process{spec: spec, exitCode: -1}
}

// Start launches the child. Returns immediately after the process is spawned
// (does not wait for it to exit). The reader for stdout/stderr is wired so
// every line lands in spec.Out as soon as the child flushes it.
func (p *Process) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil {
		return errors.New("process already started")
	}
	cmd := exec.Command(p.spec.Command, p.spec.Args...)
	cmd.Dir = p.spec.Cwd
	cmd.Env = mergeEnv(os.Environ(), p.spec.Env)
	cmd.SysProcAttr = platformSysProcAttr()

	if p.spec.Out != nil {
		// Use the same writer for both streams so the consumer sees them
		// interleaved in the natural order the process emitted them.
		cmd.Stdout = p.spec.Out
		cmd.Stderr = p.spec.Out
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd = cmd
	p.started = time.Now()
	p.done = make(chan struct{})

	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.finished = time.Now()
		if exitErr, ok := err.(*exec.ExitError); ok {
			p.exitCode = exitErr.ExitCode()
			p.exitErr = err
		} else if err != nil {
			p.exitCode = -1
			p.exitErr = err
		} else {
			p.exitCode = 0
		}
		close(p.done)
		p.mu.Unlock()
	}()

	return nil
}

// Stop tries a graceful termination first (taskkill /T on Windows,
// SIGTERM-to-process-group on macOS); waits up to `grace` for the child to
// exit; then escalates to a hard kill (taskkill /F /T on Windows, SIGKILL on
// macOS). Returns nil if the child exited within the total budget.
func (p *Process) Stop(grace time.Duration) error {
	p.mu.RLock()
	cmd := p.cmd
	done := p.done
	p.mu.RUnlock()
	if cmd == nil || cmd.Process == nil {
		return nil // never started
	}
	pid := cmd.Process.Pid

	if !p.Running() {
		return nil
	}

	if err := platformGracefulStop(pid); err != nil {
		// Even if the soft kill fails, attempt the hard one below.
	}

	if grace <= 0 {
		grace = 5 * time.Second
	}
	select {
	case <-done:
		return nil
	case <-time.After(grace):
	}

	_ = platformForceStop(pid)
	select {
	case <-done:
		return nil
	case <-time.After(3 * time.Second):
		return errors.New("process did not terminate after force-kill")
	}
}

// Running reports whether the child is still alive.
func (p *Process) Running() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// PID returns the OS-level process id once Start has been called, 0 otherwise.
func (p *Process) PID() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// ExitCode returns the child's exit code if it has finished, or -1 otherwise.
func (p *Process) ExitCode() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exitCode
}

// StartedAt returns the wall-clock time the child was spawned, or zero.
func (p *Process) StartedAt() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.started
}

// FinishedAt returns the wall-clock time the child exited, or zero.
func (p *Process) FinishedAt() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.finished
}

// ----------------------------------------------------------------------------

func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(overrides))
	used := make(map[string]bool, len(overrides))
	for _, kv := range base {
		eq := indexByte(kv, '=')
		if eq < 0 {
			out = append(out, kv)
			continue
		}
		k := kv[:eq]
		if v, ok := overrides[k]; ok {
			out = append(out, k+"="+v)
			used[k] = true
		} else {
			out = append(out, kv)
		}
	}
	for k, v := range overrides {
		if !used[k] {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
