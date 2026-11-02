package multiline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// This fake intentionally cannot claim or recover work. The attempt runner's
// dependency stays smaller than the scheduler's Repository interface.
type attemptOnlyRepository struct {
	renew  func(context.Context, Claim, time.Duration) error
	finish func(context.Context, Claim, error) (FinishResult, error)
}

func (r *attemptOnlyRepository) Renew(ctx context.Context, c Claim, d time.Duration) error {
	return r.renew(ctx, c, d)
}
func (r *attemptOnlyRepository) Finish(ctx context.Context, c Claim, err error) (FinishResult, error) {
	return r.finish(ctx, c, err)
}

func TestAttemptJoinsRenewalBeforeFinishingWithIndependentContext(t *testing.T) {
	type contextKey struct{}
	parent, cancel := context.WithCancel(context.WithValue(t.Context(), contextKey{}, "request"))
	defer cancel()
	renewStarted, renewStopped := make(chan struct{}), make(chan struct{})
	var persisted bool
	repo := &attemptOnlyRepository{
		renew: func(ctx context.Context, _ Claim, _ time.Duration) error {
			close(renewStarted)
			<-ctx.Done()
			close(renewStopped)
			return ctx.Err()
		},
		finish: func(ctx context.Context, _ Claim, executionErr error) (FinishResult, error) {
			select {
			case <-renewStopped:
			default:
				t.Error("result write overlapped active renewal")
			}
			if ctx.Err() != nil || ctx.Value(contextKey{}) != "request" {
				t.Error("completion did not preserve values independently of parent cancellation")
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Error("completion has no timeout")
			}
			if !errors.Is(executionErr, context.Canceled) {
				t.Errorf("execution error = %v", executionErr)
			}
			persisted = true
			return FinishResult{Status: StatusFailed}, nil
		},
	}
	runner := attemptRunner{
		repo: repo, leaseDuration: 30 * time.Millisecond, completionTimeout: time.Second, observe: func(Event) {},
		dispatcher: dispatcher{handlers: map[string]workerRegistration{"wait": {handler: func(ctx context.Context, _ Execution) error {
			select {
			case <-renewStarted:
				cancel()
				return nil
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				return errors.New("renewal did not start")
			}
		}}}},
	}
	if err := runner.run(parent, Claim{Execution: Execution{ID: uuid.New(), Type: "wait"}, Token: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	if !persisted {
		t.Fatal("attempt did not persist its result")
	}
}
