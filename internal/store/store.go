package store

import (
	"context"
	"fmt"

	"github.com/Edu58/multiline/internal/store/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"
)

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func New(ctx context.Context, url string) (*Store, error) {
	config, err := pgxpool.ParseConfig(url)

	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)

	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}

	return &Store{pool: pool, queries: sqlc.New(pool)}, nil
}

func (s *Store) WithTx(ctx context.Context, fn func(*sqlc.Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})

	if err != nil {
		logrus.Errorf("Error happened: %v", err)
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback(ctx)
			panic(p)
		}
	}()

	q := s.queries.WithTx(tx)

	if err := fn(q); err != nil {
		tx.Rollback(ctx)
		return fmt.Errorf("transaction failed: %w", err)
	}

	return tx.Commit(ctx)
}

func (s *Store) Close() {
	s.pool.Close()
}
