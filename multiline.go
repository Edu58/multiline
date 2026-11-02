package multiline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Options controls resource usage. Zero values select defaults.
type Options struct {
	// PollInterval is the delay after an empty claim. Default: one second.
	PollInterval time.Duration
	// CompletionTimeout bounds result persistence after handler exit. Default: five seconds.
	CompletionTimeout time.Duration
	// LeaseDuration is renewed every third of its duration. Default: 30 seconds.
	// Allowed range: 30 milliseconds to 24 hours. Use generous leases in production.
	LeaseDuration time.Duration
	// ShutdownGracePeriod allows active handlers to finish after Run's context
	// is canceled. Zero cancels immediately. Infrastructure errors bypass grace.
	ShutdownGracePeriod time.Duration
	// RecoveryInterval controls the separate bounded maintenance loop. Default: one second.
	RecoveryInterval time.Duration
	// RecoveryBatchSize bounds expired jobs processed per interval. Default: 100.
	RecoveryBatchSize int32
	// Observer receives operational events independently of handler middleware.
	Observer Observer
	// Logger optionally reports failed handler attempts.
	Logger *slog.Logger
}

// Scheduler owns no connections. Run blocks until cancellation and all handlers
// exit. A scheduler may be run again after Run returns.
type Scheduler struct {
	repo     Repository
	attempts attemptRunner
	opts     Options
	mu       sync.Mutex
	running  bool
}

// NewScheduler validates dependencies and snapshots the registry. Later
// registrations do not affect this scheduler. It starts no goroutines or timers.
func NewScheduler(repo Repository, registry *WorkerRegistry, opts Options) (*Scheduler, error) {
	if repo == nil {
		return nil, &ValidationError{Field: "repository", Reason: "is required"}
	}
	dispatch, err := registry.snapshot()
	if err != nil {
		return nil, err
	}
	if opts.PollInterval < 0 || opts.CompletionTimeout < 0 || opts.LeaseDuration < 0 || opts.ShutdownGracePeriod < 0 || opts.RecoveryInterval < 0 || opts.RecoveryBatchSize < 0 {
		return nil, &ValidationError{Field: "options", Reason: "must not be negative"}
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = time.Second
	}
	if opts.CompletionTimeout == 0 {
		opts.CompletionTimeout = 5 * time.Second
	}
	if opts.LeaseDuration == 0 {
		opts.LeaseDuration = 30 * time.Second
	}
	if opts.LeaseDuration < 30*time.Millisecond || opts.LeaseDuration > 24*time.Hour {
		return nil, &ValidationError{Field: "lease_duration", Reason: "must be between 30 milliseconds and 24 hours"}
	}
	if opts.RecoveryInterval == 0 {
		opts.RecoveryInterval = time.Second
	}
	if opts.RecoveryBatchSize == 0 {
		opts.RecoveryBatchSize = 100
	}
	if opts.RecoveryBatchSize > 1000 {
		return nil, &ValidationError{Field: "recovery_batch_size", Reason: "must be at most 1000"}
	}
	s := &Scheduler{repo: repo, opts: opts}
	s.attempts = attemptRunner{repo: repo, dispatcher: dispatch, leaseDuration: opts.LeaseDuration, completionTimeout: opts.CompletionTimeout, observe: s.observe, logger: opts.Logger}
	return s, nil
}

// Run stops claiming when ctx is canceled, allows configured shutdown grace,
// then cancels active handlers and waits for their exits and result writes.
// A handler ignoring cancellation can still delay shutdown indefinitely.
// Infrastructure failures cancel workers immediately and are returned.
func (s *Scheduler) Run(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrRunning
	}
	s.running = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.running = false; s.mu.Unlock() }()
	claimCtx, stopClaims := context.WithCancel(ctx)
	defer stopClaims()
	executionCtx, cancelExecution := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelExecution()
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	launch := func(fn func() error) {
		wg.Go(func() {
			if err := fn(); err != nil {
				select {
				case errs <- err:
				default:
				}
				stopClaims()
				cancelExecution()
			}
		})
	}
	for jobType, worker := range s.attempts.dispatcher.handlers {
		for range worker.concurrency {
			launch(func() error { return s.work(claimCtx, executionCtx, jobType) })
		}
	}
	launch(func() error { return s.recoverLoop(claimCtx) })
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		if s.opts.ShutdownGracePeriod > 0 {
			timer := time.NewTimer(s.opts.ShutdownGracePeriod)
			select {
			case <-done:
			case <-timer.C:
			}
			timer.Stop()
		}
		cancelExecution()
		<-done
	}
	select {
	case err := <-errs:
		return err
	default:
		return ctx.Err()
	}
}

func (s *Scheduler) work(claimCtx, executionCtx context.Context, jobType string) error {
	timer := time.NewTimer(s.opts.PollInterval)
	defer timer.Stop()
	for {
		if claimCtx.Err() != nil {
			return nil
		}
		started := time.Now()
		claim, err := s.repo.Claim(claimCtx, jobType, s.opts.LeaseDuration)
		if claimCtx.Err() != nil && err != nil {
			return nil
		}
		if errors.Is(err, ErrNoJob) {
			timer.Reset(s.opts.PollInterval)
			select {
			case <-claimCtx.Done():
				return nil
			case <-timer.C:
			}
			continue
		}
		if err != nil {
			s.observe(Event{Kind: EventClaimFailed, Duration: time.Since(started), Err: err})
			return fmt.Errorf("multiline: claim job: %w", err)
		}
		if claim.Execution.Type != jobType {
			err := fmt.Errorf("repository returned job type %q for worker %q", claim.Execution.Type, jobType)
			s.observe(Event{Kind: EventClaimFailed, JobID: claim.Execution.ID, Duration: time.Since(started), Err: err})
			return fmt.Errorf("multiline: claim job: %w", err)
		}
		s.observe(Event{Kind: EventClaimed, JobID: claim.Execution.ID, Attempt: claim.Execution.Attempt, Duration: time.Since(started)})
		if err := s.attempts.run(executionCtx, claim); err != nil {
			return err
		}
	}
}

func (s *Scheduler) recoverLoop(ctx context.Context) error {
	timer := time.NewTimer(s.opts.RecoveryInterval)
	defer timer.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		started := time.Now()
		recovered, err := s.repo.Recover(ctx, s.opts.RecoveryBatchSize)
		if ctx.Err() != nil && err != nil {
			return nil
		}
		if err != nil {
			s.observe(Event{Kind: EventRecoveryFailed, Duration: time.Since(started), Err: err})
			return fmt.Errorf("multiline: recover jobs: %w", err)
		}
		s.observe(Event{Kind: EventRecovered, Duration: time.Since(started), Recovery: recovered})
		timer.Reset(s.opts.RecoveryInterval)
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
	}
}
