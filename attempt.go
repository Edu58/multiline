package multiline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// attemptRepository is deliberately smaller than the scheduler's Repository:
// an attempt can maintain and finish ownership, never claim or recover work.
type attemptRepository interface {
	Renew(context.Context, Claim, time.Duration) error
	Finish(context.Context, Claim, error) (FinishResult, error)
}

// attemptRunner owns one attempt's execution, heartbeat lifetime, and result
// persistence. Its immutable configuration can be shared by scheduler workers.
// The scheduler alone coordinates intake, recovery, and shutdown grace.
type attemptRunner struct {
	repo              attemptRepository
	dispatcher        dispatcher
	leaseDuration     time.Duration
	completionTimeout time.Duration
	observe           Observer
	logger            *slog.Logger
}

func (a *attemptRunner) run(ctx context.Context, claim Claim) error {
	executionCtx, cancelExecution := context.WithCancel(ctx)
	defer cancelExecution()
	heartbeatCtx, stopHeartbeat := context.WithCancel(context.WithoutCancel(ctx))
	defer stopHeartbeat()
	heartbeatDone := make(chan error, 1)
	go func() { heartbeatDone <- a.renew(heartbeatCtx, claim, cancelExecution) }()
	executionErr := a.execute(executionCtx, claim.Execution)
	stopHeartbeat()
	if err := <-heartbeatDone; err != nil {
		if errors.Is(err, ErrLeaseLost) {
			return nil
		}
		return fmt.Errorf("multiline: renew job %s: %w", claim.Execution.ID, err)
	}
	return a.finish(ctx, claim, executionErr)
}

func (a *attemptRunner) execute(ctx context.Context, job Execution) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("handler panic: %v", p)
		}
	}()
	if job.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, job.Timeout)
		defer cancel()
	}
	err = a.dispatcher.handle(ctx, job)
	if err == nil {
		err = ctx.Err()
	}
	return err
}

func (a *attemptRunner) renew(ctx context.Context, claim Claim, cancelExecution context.CancelFunc) error {
	interval := a.leaseDuration / 3
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			started := time.Now()
			renewCtx, cancel := context.WithTimeout(ctx, interval)
			err := a.repo.Renew(renewCtx, claim, a.leaseDuration)
			cancel()
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				cancelExecution()
				kind := EventRenewFailed
				if errors.Is(err, ErrLeaseLost) {
					kind = EventLeaseLost
				}
				a.observe(Event{Kind: kind, JobID: claim.Execution.ID, Attempt: claim.Execution.Attempt, Duration: time.Since(started), Err: err})
				return err
			}
			a.observe(Event{Kind: EventRenewed, JobID: claim.Execution.ID, Attempt: claim.Execution.Attempt, Duration: time.Since(started)})
		}
	}
}

func (a *attemptRunner) finish(ctx context.Context, claim Claim, executionErr error) error {
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.completionTimeout)
	defer cancel()
	started := time.Now()
	result, err := a.repo.Finish(finishCtx, claim, executionErr)
	if errors.Is(err, ErrLeaseLost) {
		a.observe(Event{Kind: EventLeaseLost, JobID: claim.Execution.ID, Attempt: claim.Execution.Attempt, Duration: time.Since(started), Err: err})
		return nil
	}
	if err != nil {
		a.observe(Event{Kind: EventFinishFailed, JobID: claim.Execution.ID, Attempt: claim.Execution.Attempt, Duration: time.Since(started), Err: err})
		return fmt.Errorf("multiline: finish job %s: %w", claim.Execution.ID, err)
	}
	kind := EventCompleted
	if result.Status == StatusPending {
		kind = EventRetryScheduled
	} else if result.Status == StatusFailed {
		kind = EventFailed
	}
	a.observe(Event{Kind: kind, JobID: claim.Execution.ID, Attempt: claim.Execution.Attempt, Duration: time.Since(started), Err: executionErr, RetryAt: result.RetryAt})
	if executionErr != nil && a.logger != nil {
		a.logger.Error("job attempt failed", "job_id", claim.Execution.ID, "attempt", claim.Execution.Attempt, "error", executionErr)
	}
	return nil
}
