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

// GracefulStopper is an optional, opt-in capability a Service can implement
// to perform pre-stop work that protects against corruption from a hard kill.
// Registry.StopAll invokes GracefulPreStop (best-effort, errors logged but
// not fatal) before calling Stop, so the subsequent taskkill /F finds the
// on-disk state already in a recoverable shape.
//
// The poster child is HDFS NameNode: a forced checkpoint via
//   hdfs dfsadmin -safemode enter / -saveNamespace / -safemode leave
// writes VERSION + fsimage to disk so the hard kill no longer orphans
// edits_inprogress with no fsimage to recover from.
type GracefulStopper interface {
	GracefulPreStop(ctx context.Context) error
}

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
// Services that implement GracefulStopper get their pre-stop hook invoked
// first; errors at any step are collected but never abort the loop — we
// want every service to get its Stop call no matter what.
func (r *Registry) StopAll(ctx context.Context) error {
	all := r.All()
	var errs []error
	for i := len(all) - 1; i >= 0; i-- {
		svc := all[i]
		if g, ok := svc.(GracefulStopper); ok {
			if err := g.GracefulPreStop(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		if err := svc.Stop(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// StopAllWithProgress is like StopAll but calls progress on each step so the
// shutdown overlay can render a meaningful indicator. progress(step, total,
// serviceName, phase) is invoked twice per service: once for "pre-stop"
// (if GracefulStopper) and once for "stop". step is 1-indexed; total counts
// only services (not phases), so the bar advances by 1/total per service.
func (r *Registry) StopAllWithProgress(ctx context.Context, progress func(step, total int, name, phase string)) error {
	all := r.All()
	total := len(all)
	var errs []error
	for i := total - 1; i >= 0; i-- {
		svc := all[i]
		step := total - i
		if g, ok := svc.(GracefulStopper); ok {
			if progress != nil {
				progress(step, total, svc.Name(), "pre-stop")
			}
			if err := g.GracefulPreStop(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		if progress != nil {
			progress(step, total, svc.Name(), "stop")
		}
		if err := svc.Stop(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
