// Package logsink is the in-process bus that collects, retains, and broadcasts
// log lines produced by a child process. Each Sink keeps the most recent N
// lines in memory (so the UI can render history when a console is opened) and
// fans the live stream out to any number of subscribers without blocking the
// producer if a subscriber stalls.
//
// Lines are also tail-written to a per-service file under <logs>/<id>.log so
// that students can recover output from a previous run, and so that crash
// dumps survive a launcher restart.
package logsink

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Line is a single captured output line. Timestamping is done on the launcher
// side so the frontend never has to compute it.
type Line struct {
	Timestamp time.Time `json:"t"`
	Stream    string    `json:"s"` // "stdout" | "stderr"
	Text      string    `json:"x"`
	Level     string    `json:"l"` // "ERROR" | "WARN" | "INFO" (best-effort)
}

// Sink is one log channel — typically one per service.
type Sink struct {
	id       string
	capacity int

	mu      sync.RWMutex
	lines   []Line
	subs    map[int]chan Line
	nextSub int
	closed  bool

	fileMu   sync.Mutex
	file     *os.File
	syncEach bool // fsync after every line (low-volume diagnostic sinks only)
}

// SyncEachLine makes the sink fsync its log file after every written line.
// Enable it ONLY for low-volume diagnostic channels (setup, repair): on exFAT
// volumes macOS buffers unsynced writes, so another process (`cat`, a student
// tailing the file) sees an empty file until the launcher exits and the handle
// is closed. fsync makes each line durable and immediately visible. Do NOT
// enable on high-volume service sinks (hdfs/kafka/es): an fsync per line to a
// slow USB stick would throttle the producer. Call once, right after New,
// before any Emit.
func (s *Sink) SyncEachLine(v bool) { s.syncEach = v }

// New creates a Sink that retains the last `capacity` lines and tails output
// to <logsDir>/<id>.log (logsDir is created if it does not exist).
// Failures to open the file are tolerated — the in-memory bus still works.
func New(id, logsDir string, capacity int) *Sink {
	if capacity <= 0 {
		capacity = 2000
	}
	s := &Sink{
		id:       id,
		capacity: capacity,
		lines:    make([]Line, 0, capacity),
		subs:     map[int]chan Line{},
	}
	if logsDir != "" {
		if err := os.MkdirAll(logsDir, 0o755); err == nil {
			f, err := os.OpenFile(
				filepath.Join(logsDir, id+".log"),
				os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
				0o644,
			)
			if err == nil {
				s.file = f
			}
		}
	}
	return s
}

// Attach starts a goroutine that reads `r` line by line, tags each line with
// the given stream label ("stdout" | "stderr"), records it in the ring and on
// disk, and broadcasts it to subscribers. When `r` reaches EOF the goroutine
// exits. Safe to call multiple times with different streams of the same Sink.
func (s *Sink) Attach(stream string, r io.Reader) {
	go func() {
		scanner := bufio.NewScanner(r)
		// Increase the buffer because Java stack traces can be long.
		buf := make([]byte, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			s.publish(Line{
				Timestamp: time.Now(),
				Stream:    stream,
				Text:      scanner.Text(),
				Level:     detectLevel(scanner.Text()),
			})
		}
	}()
}

// Emit injects a synthetic line (e.g. a launcher-generated INFO message).
func (s *Sink) Emit(level, text string) {
	s.publish(Line{
		Timestamp: time.Now(),
		Stream:    "launcher",
		Text:      text,
		Level:     level,
	})
}

func (s *Sink) publish(ln Line) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if len(s.lines) >= s.capacity {
		// drop the oldest
		copy(s.lines, s.lines[1:])
		s.lines = s.lines[:s.capacity-1]
	}
	s.lines = append(s.lines, ln)
	// Snapshot subscribers under the lock so we can deliver without holding it.
	subs := make([]chan Line, 0, len(s.subs))
	for _, ch := range s.subs {
		subs = append(subs, ch)
	}
	s.mu.Unlock()

	// Best-effort tail-to-file. Errors are ignored — losing the on-disk log
	// must never crash the launcher.
	if s.file != nil {
		s.fileMu.Lock()
		_, _ = s.file.WriteString(formatForFile(ln) + "\n")
		if s.syncEach {
			_ = s.file.Sync()
		}
		s.fileMu.Unlock()
	}

	// Non-blocking fan-out. If a subscriber is full, drop. The Snapshot()
	// call on reconnect lets it catch up.
	for _, ch := range subs {
		select {
		case ch <- ln:
		default:
		}
	}
}

// Snapshot returns a copy of the currently retained lines. The slice is safe
// for the caller to mutate.
func (s *Sink) Snapshot() []Line {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Line, len(s.lines))
	copy(out, s.lines)
	return out
}

// Subscribe registers a new live-stream listener and returns its id and the
// channel. The channel buffer is intentionally small (128) — slow subscribers
// drop lines rather than back-pressure the producer.
func (s *Sink) Subscribe() (int, <-chan Line) {
	ch := make(chan Line, 128)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		close(ch)
		return -1, ch
	}
	id := s.nextSub
	s.nextSub++
	s.subs[id] = ch
	return id, ch
}

// Unsubscribe removes a listener and closes its channel.
func (s *Sink) Unsubscribe(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.subs[id]; ok {
		delete(s.subs, id)
		close(ch)
	}
}

// Clear empties the ring buffer (but keeps subscribers and the file open).
func (s *Sink) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = s.lines[:0]
}

// Close stops accepting new lines, closes the file, and closes every
// subscriber channel. Idempotent.
func (s *Sink) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	subs := s.subs
	s.subs = map[int]chan Line{}
	s.mu.Unlock()

	for _, ch := range subs {
		close(ch)
	}

	s.fileMu.Lock()
	defer s.fileMu.Unlock()
	if s.file != nil {
		err := s.file.Close()
		s.file = nil
		return err
	}
	return nil
}

// ----------------------------------------------------------------------------
// helpers

var (
	reError = regexp.MustCompile(`(?i)\b(ERROR|FATAL|SEVERE|Exception|Traceback)\b`)
	reWarn  = regexp.MustCompile(`(?i)\b(WARN(ING)?)\b`)
)

func detectLevel(text string) string {
	switch {
	case reError.MatchString(text):
		return "ERROR"
	case reWarn.MatchString(text):
		return "WARN"
	default:
		return "INFO"
	}
}

func formatForFile(ln Line) string {
	var sb strings.Builder
	sb.WriteString(ln.Timestamp.Format("2006-01-02 15:04:05.000"))
	sb.WriteByte(' ')
	sb.WriteString(ln.Level)
	sb.WriteByte(' ')
	sb.WriteString(ln.Stream)
	sb.WriteString(" | ")
	sb.WriteString(ln.Text)
	return sb.String()
}
