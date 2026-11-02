package postgres_test

import (
	"context"
	"log"
	"time"

	"github.com/Edu58/multiline"
	"github.com/Edu58/multiline/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ExampleNewTx() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, "postgresql://localhost/multiline?sslmode=disable")
	if err != nil {
		log.Print(err)
		return
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		log.Print(err)
		return
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanupCtx) // Safe after Commit as well.
	}()

	// Perform application writes through tx here. The enqueue will commit or
	// roll back together with those writes.
	jobs := postgres.NewTx(tx)
	if _, err := jobs.Enqueue(ctx, multiline.EnqueueParams{Type: "email", RunAt: time.Now()}); err != nil {
		log.Print(err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		log.Print(err)
	}
}
