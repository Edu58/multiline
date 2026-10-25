package workers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

type Job struct {
	Id         uuid.UUID
	JobType    string
	Payload    json.RawMessage
	Expiration int64
}

type Handler interface {
	HandleJob(ctx context.Context, j *Job, opts ...any) error                   // Executes the job
	CancelJob(ctx context.Context, j *Job, reason []byte, opts ...any) error    // Cancels the job
	TerminateJob(ctx context.Context, j *Job, reason []byte, opts ...any) error // Terminates a running job
}

type Worker struct {
	handler Handler
}

func NewWorker(h Handler) (*Worker, error) {
	if h == nil {
		return nil, errors.New("job handler cannot be nil")
	}

	return &Worker{handler: h}, nil
}

func (w *Worker) HandleJob(ctx context.Context, j *Job, opts ...any) error {
	return w.handler.HandleJob(ctx, j, opts...)
}

func (w *Worker) CancelJob(ctx context.Context, j *Job, reason []byte, opts ...any) error {
	return w.handler.CancelJob(ctx, j, reason, opts...)
}

func (w *Worker) TerminateJob(ctx context.Context, j *Job, reason []byte, opts ...any) error {
	return w.handler.TerminateJob(ctx, j, reason, opts...)
}
