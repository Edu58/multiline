package workers

import (
	"context"
	"errors"
	"fmt"

	"github.com/Edu58/multiline/config"
)

type Dispatcher struct {
	config   *config.Config
	registry *WorkerRegistry
}

func NewDispatcher(c *config.Config, r *WorkerRegistry) (*Dispatcher, error) {
	if r == nil {
		return nil, errors.New("registry cannot be nil")
	}

	if c == nil {
		return nil, errors.New("config cannot be nil")
	}

	for _, name := range c.WORKERS {
		if _, exists := r.workers[name]; !exists {
			return nil, fmt.Errorf("worker %s not registered", name)
		}

	}

	return &Dispatcher{config: c, registry: r}, nil
}

func (d *Dispatcher) HandleJob(ctx context.Context, j *Job, opts ...any) error {
	handlerName, exists := d.config.WORKERS[j.JobType]

	if !exists {
		return fmt.Errorf("error getting handler for job type %s", j.JobType)
	}

	h, err := d.registry.Get(handlerName)

	if err != nil {
		return err
	}

	return h.HandleJob(ctx, j, opts...)
}
