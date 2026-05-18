// Package services defines the lifecycle contract that every BDP service
// (Elasticsearch, Kafka, HDFS NameNode, HDFS DataNode, Jupyter) implements
// and the Registry that the App layer talks to.
//
// A Service is responsible for: spawning its process via processctl,
// surfacing live output through a logsink.Sink, and computing its health
// from a probe (TCP or HTTP). The Registry stitches these together and
// exposes a uniform API to the JS frontend.
package services

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/abxda/bdpv6-launcher/internal/logsink"
)

// Service is the contract every BDP service implementation satisfies.
// Implementations live in their own files (elasticsearch.go, kafka.go, ...).
type Service interface {
	// Stable machine id used as a key in the registry and event names.
	ID() string

	// Human label for the UI (es-MX preferred).
	Name() string

	// The port the service listens on (after applying any override).
	Port() int

	// Start the service in the background. Idempotent: if already running,
	// returns nil without spawning a second copy.
	Start(ctx context.Context) error

	// Stop the service if running. Blocks until the process exits or the
	// hard-kill window elapses. Idempotent.
	Stop(ctx context.Context) error

	// Status snapshot for the UI. Must be cheap (it runs on every tick).
	Status() Status

	// The shared log bus where this service publishes stdout/stderr lines.
	Logs() *logsink.Sink
}

// Status is what the UI renders next to a service: are we running, are we
// healthy (probe succeeded), what is the PID, since when, and any short
// detail message for tooltips.
type Status struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Port     int       `json:"port"`
	Running  bool      `json:"running"`
	Healthy  bool      `json:"healthy"`
	PID      int       `json:"pid"`
	Since    time.Time `json:"since,omitempty"`
	Detail   string    `json:"detail"`
	ExitCode int       `json:"exitCode"`
}

// Registry owns the live set of services and orders them deterministically
// for UI listings. Construct with NewRegistry, populate with Register, then
// look up via Get/All. Safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	items map[string]Service
	order []string
}

func NewRegistry() *Registry {
	return &Registry{items: map[string]Service{}}
}

// Register adds a service. If a service with the same ID already exists it
// is replaced (useful during the F5 first-run wizard which rebuilds the
// registry after port edits).
func (r *Registry) Register(s Service) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.items[s.ID()]; !dup {
		r.order = append(r.order, s.ID())
	}
	r.items[s.ID()] = s
}

// Get looks up a service by id.
func (r *Registry) Get(id string) (Service, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.items[id]
	return s, ok
}

// All returns the services in registration order. Safe for the caller to
// iterate without holding the lock; the slice is a fresh copy.
func (r *Registry) All() []Service {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Service, 0, len(r.order))
	for _, id := range r.order {
		if s, ok := r.items[id]; ok {
			out = append(out, s)
		}
	}
	return out
}

// AllStatuses returns a map keyed by service id. Used by the dashboard tick.
func (r *Registry) AllStatuses() map[string]Status {
	all := r.All()
	out := make(map[string]Status, len(all))
	for _, s := range all {
		out[s.ID()] = s.Status()
	}
	return out
}

// SortedStatuses returns statuses in registration order — useful for the UI.
func (r *Registry) SortedStatuses() []Status {
	all := r.All()
	out := make([]Status, 0, len(all))
	for _, s := range all {
		out = append(out, s.Status())
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// StopAll shuts down every registered service in reverse registration order
// (e.g. Jupyter before HDFS so notebooks lose their session cleanly first).
// Errors are collected but do not abort the loop.
func (r *Registry) StopAll(ctx context.Context) error {
	all := r.All()
	var errs []error
	for i := len(all) - 1; i >= 0; i-- {
		if err := all[i].Stop(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
