package customhandlers

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Edu58/multiline/internal/store"
	"github.com/Edu58/multiline/internal/store/sqlc"
	"github.com/Edu58/multiline/internal/workers"
	"github.com/jackc/pgx/v5/pgtype"
)

type EmailWorker struct{}

func NewEmailWorker() *EmailWorker {
	return &EmailWorker{}
}

func (e *EmailWorker) HandleJob(ctx context.Context, j *workers.Job, opts ...any) error {
	store, err := findStore(opts...)

	if err != nil {
		return err
	}

	if err := store.Queries.UpdateJobStartedAt(ctx, sqlc.UpdateJobStartedAtParams{
		ID:        j.Id,
		StartedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		Status:    new("complete"),
	}); err != nil {
		return err
	}

	log.Println("SUCCESSFULLY RAN FIRST JOB")

	return nil
}

func (e *EmailWorker) CancelJob(_ context.Context, j *workers.Job, _ []byte, opts ...any) error {
	log.Printf("Job %v cancelled \n", j.Id)
	return nil
}

func (e *EmailWorker) TerminateJob(_ context.Context, j *workers.Job, _ []byte, opts ...any) error {
	log.Printf("Job %v terminated \n", j.Id)

	return nil
}

func findStore(opts ...any) (*store.Store, error) {
	for _, i := range opts {
		if store, ok := i.(*store.Store); ok {
			if store != nil {
				return store, nil
			}

			return store, errors.New("store is nil")
		}
	}

	return nil, errors.New("store not found in opts")
}
