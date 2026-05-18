// Package state persists user preferences and setup progress to a JSON file
// next to the launcher binary (.bdp_state.json). Concurrent access is safe;
// callers obtain a copy via Get() and apply atomic mutations via Update().
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State is the on-disk shape. New fields should be added with sensible zero
// values so older state files still load without migration logic.
type State struct {
	SetupCompleted   bool            `json:"setupCompleted"`
	SetupCompletedAt time.Time       `json:"setupCompletedAt,omitempty"`
	Language         string          `json:"language"`
	AlwaysOnTop      bool            `json:"alwaysOnTop"`
	AutoStartJupyter bool            `json:"autoStartJupyter"`
	PortOverrides    map[string]int  `json:"portOverrides"`
	JVMHeap          map[string]string `json:"jvmHeap"`
}

func defaults() State {
	return State{
		Language:         "es",
		AlwaysOnTop:      false,
		AutoStartJupyter: true,
		PortOverrides:    map[string]int{},
		JVMHeap: map[string]string{
			"elasticsearch": "1g",
			"hadoop":        "1g",
		},
	}
}

type Store struct {
	path string
	mu   sync.RWMutex
	cur  State
}

func NewStore(path string) *Store {
	return &Store{path: path, cur: defaults()}
}

// Load reads the JSON file from disk into memory. If the file does not exist,
// defaults are kept and no error is returned (this is the expected first-run
// case). Malformed JSON is reported as an error but defaults are retained.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var loaded State
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}
	// Merge into defaults so newly added fields keep their zero defaults
	// instead of inheriting null/empty maps from older state files.
	merged := defaults()
	if loaded.Language != "" {
		merged.Language = loaded.Language
	}
	merged.SetupCompleted = loaded.SetupCompleted
	merged.SetupCompletedAt = loaded.SetupCompletedAt
	merged.AlwaysOnTop = loaded.AlwaysOnTop
	merged.AutoStartJupyter = loaded.AutoStartJupyter
	if loaded.PortOverrides != nil {
		merged.PortOverrides = loaded.PortOverrides
	}
	if loaded.JVMHeap != nil {
		for k, v := range loaded.JVMHeap {
			merged.JVMHeap[k] = v
		}
	}
	s.cur = merged
	return nil
}

// Save writes the current state to disk atomically (write-temp + rename).
func (s *Store) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.cur, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	tmp, err := os.CreateTemp(dir, ".bdp_state.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, s.path)
}

// Get returns a value-copy of the current state. Safe to share with callers.
func (s *Store) Get() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Update applies a mutator under the write lock and persists immediately.
// Returns the save error so callers can surface it (e.g. permission denied).
func (s *Store) Update(mutate func(*State)) error {
	s.mu.Lock()
	mutate(&s.cur)
	s.mu.Unlock()
	return s.Save()
}

// MarkSetupCompleted records that the first-run wizard finished successfully.
func (s *Store) MarkSetupCompleted() error {
	return s.Update(func(st *State) {
		st.SetupCompleted = true
		st.SetupCompletedAt = time.Now()
	})
}
