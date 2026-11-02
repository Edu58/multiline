package multiline_test

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/Edu58/multiline"
)

func ExampleChain() {
	// Replace report with an application metrics or tracing implementation.
	report := func(jobType string, elapsed time.Duration, err error) {
		fmt.Printf("type=%s success=%t\n", jobType, err == nil)
	}
	timing := func(next multiline.Handler) multiline.Handler {
		return func(ctx context.Context, execution multiline.Execution) (err error) {
			started := time.Now()
			defer func() {
				if p := recover(); p != nil {
					report(execution.Type, time.Since(started), fmt.Errorf("handler panic: %v", p))
					panic(p) // Preserve the scheduler's panic containment and failed result.
				}
				report(execution.Type, time.Since(started), err)
			}()
			return next(ctx, execution)
		}
	}
	handler := multiline.Chain(func(context.Context, multiline.Execution) error { return nil }, timing)
	_ = handler(context.Background(), multiline.Execution{Type: "email"})
	// Output: type=email success=true
}

func ExampleObserver() {
	// Send bounded-cardinality event kinds to your own metrics implementation.
	// Synchronize shared state because workers and recovery invoke this concurrently.
	var recovered atomic.Int64
	observer := multiline.Observer(func(event multiline.Event) {
		if event.Kind == multiline.EventRecovered {
			recovered.Add(event.Recovery.Retried + event.Recovery.Failed)
		}
		if event.Kind == multiline.EventFinishFailed || event.Kind == multiline.EventLeaseLost {
			log.Printf("job %s operation %s: %v", event.JobID, event.Kind, event.Err)
		}
	})
	options := multiline.Options{Observer: observer}
	_ = options // Pass these options to NewScheduler.
}
