package multiline

import (
	"fmt"
	"sync"
)

// WorkerOptions controls a handler's dedicated pool within one scheduler.
// Concurrency zero defaults to one. Limits are per scheduler instance, not
// coordinated across processes; each worker claims only its registered job type.
type WorkerOptions struct{ Concurrency int }

type workerRegistration struct {
	handler     Handler
	concurrency int
}

// WorkerRegistry associates job types directly with Handler functions. Its zero
// value is usable. Register and scheduler construction are safe concurrently;
// a registry must not be copied after first use. Each scheduler snapshots its
// registrations, so later additions affect only subsequently created schedulers.
// Handler functions themselves must support concurrent use.
type WorkerRegistry struct {
	mu       sync.RWMutex
	handlers map[string]workerRegistration
}

// NewWorkerRegistry creates an empty registry. Register at least one handler
// before passing it to NewScheduler.
func NewWorkerRegistry() *WorkerRegistry { return &WorkerRegistry{} }

// Register associates jobType with handler. Invalid names or nil handlers return
// ErrInvalid; duplicate names return ErrDuplicateHandler without replacing the
// existing handler. Job types are case-sensitive and use the enqueue type rules.
// Omit options (or use zero values) for one worker; a positive Concurrency
// reserves that many workers for this job type. Negative values are invalid.
func (r *WorkerRegistry) Register(jobType string, handler Handler, options ...WorkerOptions) error {
	if r == nil {
		return invalid("registry", "is required")
	}
	if err := ValidateJobType(jobType); err != nil {
		return err
	}
	if handler == nil {
		return invalid("handler", "is required")
	}
	if len(options) > 1 {
		return invalid("worker_options", "at most one options value is allowed")
	}
	concurrency := 1
	if len(options) == 1 {
		if options[0].Concurrency < 0 {
			return invalid("concurrency", "must be nonnegative; zero defaults to one")
		}
		if options[0].Concurrency > 0 {
			concurrency = options[0].Concurrency
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.handlers[jobType]; exists {
		return fmt.Errorf("handler %q: %w", jobType, ErrDuplicateHandler)
	}
	if r.handlers == nil {
		r.handlers = make(map[string]workerRegistration)
	}
	r.handlers[jobType] = workerRegistration{handler: handler, concurrency: concurrency}
	return nil
}

func (r *WorkerRegistry) snapshot() (dispatcher, error) {
	if r == nil {
		return dispatcher{}, invalid("registry", "is required")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.handlers) == 0 {
		return dispatcher{}, invalid("registry", "at least one handler is required")
	}
	copied := make(map[string]workerRegistration, len(r.handlers))
	for name, handler := range r.handlers {
		copied[name] = handler
	}
	return dispatcher{handlers: copied}, nil
}
