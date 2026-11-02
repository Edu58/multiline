package multiline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type memoryRepository struct {
	mu        sync.Mutex
	jobs      []Execution
	results   map[uuid.UUID]error
	claimErr  error
	finishErr error
}

func (r *memoryRepository) Claim(ctx context.Context, _ time.Duration) (Claim, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.claimErr != nil {
		return Claim{}, r.claimErr
	}
	if len(r.jobs) == 0 {
		return Claim{}, ErrNoJob
	}
	j := r.jobs[0]
	r.jobs = r.jobs[1:]
	return Claim{Execution: j, Token: uuid.New()}, nil
}
func (r *memoryRepository) Recover(context.Context, int32) (RecoveryResult, error) {
	return RecoveryResult{}, nil
}
func (r *memoryRepository) Renew(context.Context, Claim, time.Duration) error { return nil }
func (r *memoryRepository) Finish(ctx context.Context, claim Claim, err error) (FinishResult, error) {
	if ctx.Err() != nil {
		return FinishResult{}, ctx.Err()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results[claim.Execution.ID] = err
	status := StatusCompleted
	if err != nil {
		status = StatusFailed
	}
	return FinishResult{Status: status}, r.finishErr
}
func TestRunCompletesAndContainsPanics(t *testing.T) {
	for _, typ := range []string{"ok", "panic", "missing", "timeout"} {
		t.Run(typ, func(t *testing.T) {
			id := uuid.New()
			repo := &memoryRepository{jobs: []Execution{{ID: id, Type: typ, Timeout: time.Millisecond}}, results: make(map[uuid.UUID]error)}
			scheduler, err := NewScheduler(repo, map[string]Handler{
				"ok":      func(context.Context, Execution) error { return nil },
				"panic":   func(context.Context, Execution) error { panic("boom") },
				"timeout": func(ctx context.Context, _ Execution) error { <-ctx.Done(); return nil },
			}, Options{PollInterval: time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- scheduler.Run(ctx) }()
			deadline := time.NewTimer(time.Second)
			defer deadline.Stop()
			for {
				repo.mu.Lock()
				result, found := repo.results[id]
				repo.mu.Unlock()
				if found {
					if (result != nil) != (typ != "ok") {
						t.Fatalf("result = %v", result)
					}
					break
				}
				select {
				case <-deadline.C:
					t.Fatal("job never finished")
				case <-time.After(time.Millisecond):
				}
			}
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("Run = %v", err)
			}
		})
	}
}
func TestRunReturnsRepositoryError(t *testing.T) {
	sentinel := errors.New("database unavailable")
	for _, finish := range []bool{false, true} {
		repo := &memoryRepository{results: make(map[uuid.UUID]error)}
		if finish {
			repo.jobs = []Execution{{ID: uuid.New(), Type: "ok"}}
			repo.finishErr = sentinel
		} else {
			repo.claimErr = sentinel
		}
		s, err := NewScheduler(repo, map[string]Handler{"ok": func(context.Context, Execution) error { return nil }}, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Run(t.Context()); !errors.Is(err, sentinel) {
			t.Fatalf("Run = %v", err)
		}
	}
}
func TestRunWaitsForHandlerAndRejectsConcurrentRun(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	repo := &memoryRepository{jobs: []Execution{{ID: uuid.New(), Type: "wait"}}, results: make(map[uuid.UUID]error)}
	s, err := NewScheduler(repo, map[string]Handler{"wait": func(ctx context.Context, _ Execution) error { close(started); <-release; return ctx.Err() }}, Options{Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	<-started
	if err := s.Run(t.Context()); !errors.Is(err, ErrRunning) {
		t.Fatalf("Run = %v", err)
	}
	cancel()
	select {
	case <-done:
		t.Fatal("returned before handler finished")
	default:
	}
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.results) != 1 {
		t.Fatal("canceled execution was not persisted")
	}
}
func TestNewSchedulerValidation(t *testing.T) {
	repo := &memoryRepository{}
	h := map[string]Handler{"ok": func(context.Context, Execution) error { return nil }}
	for _, tc := range []struct {
		repo     Repository
		handlers map[string]Handler
		opts     Options
	}{
		{nil, h, Options{}}, {repo, nil, Options{}}, {repo, map[string]Handler{"bad": nil}, Options{}}, {repo, h, Options{Concurrency: -1}},
	} {
		if _, err := NewScheduler(tc.repo, tc.handlers, tc.opts); !errors.Is(err, ErrInvalid) {
			t.Fatalf("validation error = %v", err)
		} else {
			var detail *ValidationError
			if !errors.As(err, &detail) || detail.Field == "" {
				t.Fatalf("missing validation detail: %v", err)
			}
		}
	}
}

func TestConcurrencyBound(t *testing.T) {
	repo := &memoryRepository{results: make(map[uuid.UUID]error)}
	for range 12 {
		repo.jobs = append(repo.jobs, Execution{ID: uuid.New(), Type: "wait"})
	}
	started := make(chan struct{}, 12)
	release := make(chan struct{})
	s, err := NewScheduler(repo, map[string]Handler{"wait": func(context.Context, Execution) error { started <- struct{}{}; <-release; return nil }}, Options{Concurrency: 3})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	for range 3 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start")
		}
	}
	repo.mu.Lock()
	remaining := len(repo.jobs)
	repo.mu.Unlock()
	if remaining != 9 {
		t.Fatalf("claimed too many jobs: %d remaining", remaining)
	}
	cancel()
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

type renewingRepository struct {
	*memoryRepository
	renew func(context.Context, Claim, time.Duration) error
}

func (r *renewingRepository) Renew(ctx context.Context, claim Claim, lease time.Duration) error {
	return r.renew(ctx, claim, lease)
}

func TestLeaseRenewalCancelsHandlerOnLossOrFailure(t *testing.T) {
	infrastructureErr := errors.New("renewal unavailable")
	for _, renewErr := range []error{ErrLeaseLost, infrastructureErr} {
		t.Run(renewErr.Error(), func(t *testing.T) {
			base := &memoryRepository{results: make(map[uuid.UUID]error)}
			repo := &renewingRepository{memoryRepository: base, renew: func(context.Context, Claim, time.Duration) error { return renewErr }}
			handler := func(ctx context.Context, _ Execution) error { <-ctx.Done(); return ctx.Err() }
			s, err := NewScheduler(repo, map[string]Handler{"wait": handler}, Options{LeaseDuration: 30 * time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			err = s.attempts.run(ctx, Claim{Execution: Execution{ID: uuid.New(), Type: "wait"}, Token: uuid.New()})
			if errors.Is(renewErr, ErrLeaseLost) {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, infrastructureErr) {
				t.Fatalf("process = %v", err)
			}
			if ctx.Err() != nil {
				t.Fatal("handler was not canceled by renewal failure")
			}
			if len(base.results) != 0 {
				t.Fatal("persisted result after ownership became uncertain")
			}
		})
	}
}

func TestLeaseRenewalContinuesDuringHandler(t *testing.T) {
	renewed := make(chan struct{}, 4)
	base := &memoryRepository{results: make(map[uuid.UUID]error)}
	repo := &renewingRepository{memoryRepository: base, renew: func(context.Context, Claim, time.Duration) error { renewed <- struct{}{}; return nil }}
	s, err := NewScheduler(repo, map[string]Handler{"wait": func(ctx context.Context, _ Execution) error {
		for range 2 {
			select {
			case <-renewed:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}}, Options{LeaseDuration: 30 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	claim := Claim{Execution: Execution{ID: uuid.New(), Type: "wait"}, Token: uuid.New()}
	if err := s.attempts.run(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if result, ok := base.results[claim.Execution.ID]; !ok || result != nil {
		t.Fatalf("result = %v, persisted=%v", result, ok)
	}
}

type recoveringRepository struct {
	*memoryRepository
	recover func(context.Context, int32) (RecoveryResult, error)
}

func (r *recoveringRepository) Recover(ctx context.Context, limit int32) (RecoveryResult, error) {
	return r.recover(ctx, limit)
}

type finishingRepository struct {
	*memoryRepository
	finish func(context.Context, Claim, error) (FinishResult, error)
}

func (r *finishingRepository) Finish(ctx context.Context, c Claim, err error) (FinishResult, error) {
	return r.finish(ctx, c, err)
}

func TestGracefulShutdownDrainsThenCancels(t *testing.T) {
	for _, finishDuringGrace := range []bool{true, false} {
		t.Run(fmt.Sprint(finishDuringGrace), func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			id := uuid.New()
			repo := &memoryRepository{jobs: []Execution{{ID: id, Type: "wait"}, {ID: uuid.New(), Type: "wait"}}, results: make(map[uuid.UUID]error)}
			var handlerCanceled bool
			s, err := NewScheduler(repo, map[string]Handler{"wait": func(ctx context.Context, _ Execution) error {
				close(started)
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					handlerCanceled = true
					return ctx.Err()
				}
			}}, Options{Concurrency: 1, ShutdownGracePeriod: 100 * time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- s.Run(ctx) }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("handler never started")
			}
			cancel()
			if finishDuringGrace {
				close(release)
			}
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("shutdown hung")
			}
			if handlerCanceled == finishDuringGrace {
				t.Fatalf("canceled=%v, finish during grace=%v", handlerCanceled, finishDuringGrace)
			}
			repo.mu.Lock()
			defer repo.mu.Unlock()
			if len(repo.jobs) != 1 {
				t.Fatal("accepted another job during shutdown")
			}
			result, ok := repo.results[id]
			if !ok || (result != nil) == finishDuringGrace {
				t.Fatalf("persisted result = %v, found=%v", result, ok)
			}
		})
	}
}

func TestRecoveryFailureBypassesShutdownGrace(t *testing.T) {
	started := make(chan struct{})
	sentinel := errors.New("recovery unavailable")
	repo := &recoveringRepository{
		memoryRepository: &memoryRepository{jobs: []Execution{{ID: uuid.New(), Type: "wait"}}, results: make(map[uuid.UUID]error)},
		recover: func(ctx context.Context, _ int32) (RecoveryResult, error) {
			select {
			case <-started:
				return RecoveryResult{}, sentinel
			case <-ctx.Done():
				return RecoveryResult{}, ctx.Err()
			}
		},
	}
	s, err := NewScheduler(repo, map[string]Handler{"wait": func(ctx context.Context, _ Execution) error { close(started); <-ctx.Done(); return ctx.Err() }}, Options{Concurrency: 1, ShutdownGracePeriod: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := s.Run(ctx); !errors.Is(err, sentinel) {
		t.Fatalf("Run = %v", err)
	}
	if ctx.Err() != nil {
		t.Fatal("infrastructure failure waited for grace")
	}
}

func TestPersistedOutcomeEvents(t *testing.T) {
	for _, tc := range []struct {
		result     FinishResult
		persistErr error
		kind       EventKind
	}{
		{FinishResult{Status: StatusCompleted}, nil, EventCompleted},
		{FinishResult{Status: StatusPending}, nil, EventRetryScheduled},
		{FinishResult{Status: StatusFailed}, nil, EventFailed},
		{FinishResult{}, ErrLeaseLost, EventLeaseLost},
		{FinishResult{}, errors.New("storage unavailable"), EventFinishFailed},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			repo := &finishingRepository{memoryRepository: &memoryRepository{}, finish: func(context.Context, Claim, error) (FinishResult, error) { return tc.result, tc.persistErr }}
			var events []Event
			s, err := NewScheduler(repo, map[string]Handler{"ok": func(context.Context, Execution) error { return nil }}, Options{Observer: func(e Event) { events = append(events, e) }})
			if err != nil {
				t.Fatal(err)
			}
			id := uuid.New()
			err = s.attempts.run(t.Context(), Claim{Execution: Execution{ID: id, Type: "ok", Attempt: 2}, Token: uuid.New()})
			if tc.persistErr != nil && !errors.Is(tc.persistErr, ErrLeaseLost) && !errors.Is(err, tc.persistErr) {
				t.Fatal(err)
			}
			if len(events) != 1 || events[0].Kind != tc.kind || events[0].JobID != id || events[0].Attempt != 2 {
				t.Fatalf("events = %+v", events)
			}
		})
	}
}

func TestObserverPanicDoesNotFailExecution(t *testing.T) {
	repo := &memoryRepository{results: make(map[uuid.UUID]error)}
	s, err := NewScheduler(repo, map[string]Handler{"ok": func(context.Context, Execution) error { return nil }}, Options{Observer: func(Event) { panic("telemetry bug") }})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.attempts.run(t.Context(), Claim{Execution: Execution{ID: uuid.New(), Type: "ok"}}); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownHandlerIsPermanent(t *testing.T) {
	s, err := NewScheduler(&memoryRepository{}, map[string]Handler{"known": func(context.Context, Execution) error { return nil }}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.attempts.execute(t.Context(), Execution{Type: "unknown"}); !IsPermanent(err) {
		t.Fatalf("unknown handler = %v", err)
	}
}
