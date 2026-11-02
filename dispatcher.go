package multiline

import (
	"context"
	"fmt"
)

// dispatcher resolves job types from an immutable registry snapshot. It owns no
// lifecycle policy: timeouts, panic containment, leases, and result persistence
// belong to the attempt runner.
type dispatcher struct{ handlers map[string]workerRegistration }

func (d dispatcher) handle(ctx context.Context, execution Execution) error {
	worker, ok := d.handlers[execution.Type]
	if !ok {
		return Permanent(fmt.Errorf("unregistered job type %q", execution.Type))
	}
	return worker.handler(ctx, execution)
}
