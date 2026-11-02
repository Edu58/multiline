package multiline_test

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/Edu58/multiline"
	"github.com/Edu58/multiline/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ExampleNewScheduler() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // In an application, use signal.NotifyContext for shutdown.
	pool, err := pgxpool.New(ctx, "postgresql://localhost/multiline?sslmode=disable")
	if err != nil {
		log.Print(err)
		return
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Print(err)
		return
	}
	db := postgres.New(pool)
	scheduler, err := multiline.NewScheduler(db, map[string]multiline.Handler{
		"email": func(ctx context.Context, job multiline.Execution) error {
			// Send using a context-aware client and job.ID as an idempotency key.
			return nil
		},
	}, multiline.Options{Concurrency: 8, PollInterval: time.Second, ShutdownGracePeriod: 30 * time.Second})
	if err != nil {
		log.Print(err)
		return
	}
	if err := scheduler.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Print(err)
	}
}
