// Package postgres implements multiline persistence using PostgreSQL and sqlc.
// Applications own connections, migrations, and transaction lifecycles.
package postgres

import (
	"github.com/Edu58/multiline/postgres/internal/dbgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// access exposes only enqueue and inspection. Embedding it does not give a
// transaction the claiming, renewal, or recovery methods required by Repository.
type access struct{ queries *dbgen.Queries }

// Store is a concurrent, pool-backed Repository. Its zero value is unusable.
// Construct it with New; generated types remain internal.
type Store struct{ *access }

// TxStore supports transactional enqueue and inspection. It deliberately does
// not implement multiline.Repository, preventing its use by a Scheduler.
// The caller owns commit/rollback and must not use it concurrently.
type TxStore struct{ *access }

// New binds a store to an application-owned, non-nil pool without I/O. The caller
// verifies connectivity and closes the pool after its schedulers stop.
func New(pool *pgxpool.Pool) *Store { return &Store{access: &access{queries: dbgen.New(pool)}} }

// NewTx binds enqueue and inspection to a non-nil application transaction.
// The caller must commit or roll back; the store never ends the transaction.
func NewTx(tx pgx.Tx) *TxStore { return &TxStore{access: &access{queries: dbgen.New(tx)}} }
