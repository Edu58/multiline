package workers

import (
	"errors"
	"fmt"
)

type WorkerRegistry struct {
	workers map[string]Handler
}

func NewWorkerRegistry() *WorkerRegistry {
	return &WorkerRegistry{workers: make(map[string]Handler)}
}

func (wr *WorkerRegistry) Register(name string, worker Handler) error {
	if name == "" {
		return errors.New("name must be provided and should match to a job type")
	}

	if worker == nil {
		return errors.New("handler cannot be nil")
	}

	if _, exists := wr.workers[name]; exists {
		return fmt.Errorf("worker with name %s already registered", name)
	}

	wr.workers[name] = worker
	return nil
}

func (wr *WorkerRegistry) Get(name string) (Handler, error) {
	if worker, exists := wr.workers[name]; exists {
		return worker, nil
	}

	return nil, fmt.Errorf("worker with name %s not registered", name)
}

func (wr *WorkerRegistry) Delete(name string) error {
	if _, exists := wr.workers[name]; exists {
		delete(wr.workers, name)
		return nil
	}

	return fmt.Errorf("worker with name %s not registered", name)
}
