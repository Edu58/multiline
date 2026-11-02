//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Edu58/multiline"
	"github.com/Edu58/multiline/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newTestPool isolates every test from other queues and cleans up after failure.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return newSchemaPool(t, true)
}

func newSchemaPool(t *testing.T, clone bool) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("MULTILINE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MULTILINE_TEST_DSN is required for integration tests")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := "test_" + uuid.New().String()[:8]
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if clone {
		contents, err := os.ReadFile("../db/migrations/000001_jobs.up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(contents)); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

type lister interface {
	List(context.Context, multiline.ListParams) (multiline.JobPage, error)
}

func listJobs(ctx context.Context, db lister, limit int32) ([]multiline.JobRecord, error) {
	page, err := db.List(ctx, multiline.ListParams{Limit: limit})
	return page.Jobs, err
}

func registered(t *testing.T, handlers map[string]multiline.Handler) *multiline.WorkerRegistry {
	t.Helper()
	registry := multiline.NewWorkerRegistry()
	for name, handler := range handlers {
		if err := registry.Register(name, handler); err != nil {
			t.Fatal(err)
		}
	}
	return registry
}

func TestConcurrentClaims(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	db := postgres.New(pool)
	const count = 32
	for range count {
		if _, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now().Add(-time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	ids := make(chan uuid.UUID, count)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for {
				job, err := db.Claim(ctx, "test", time.Minute)
				if errors.Is(err, multiline.ErrNoJob) {
					return
				}
				if err != nil {
					t.Error(err)
					return
				}
				select {
				case ids <- job.Execution.ID:
				default:
					t.Error("too many claims")
					return
				}
				if _, err := db.Finish(ctx, job, nil); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
	close(ids)
	seen := make(map[uuid.UUID]bool)
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate claim %s", id)
		}
		seen[id] = true
	}
	if len(seen) != count {
		t.Fatalf("claimed %d jobs, want %d", len(seen), count)
	}
	var completed int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE status='completed'").Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if completed != count {
		t.Fatalf("completed %d jobs", completed)
	}
}

func TestRoundTripAndTerminalStates(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	db := postgres.New(pool)
	if _, err := db.Claim(ctx, "test", time.Minute); !errors.Is(err, multiline.ErrNoJob) {
		t.Fatalf("empty claim: %v", err)
	}
	when := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Second)
	payload := []byte(`{"message":"hello"}`)
	id, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: when, Payload: payload, Timeout: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Enqueue(ctx, multiline.EnqueueParams{ID: id, Type: "test", RunAt: when}); !errors.Is(err, multiline.ErrDuplicateJob) {
		t.Fatalf("duplicate ID error = %v", err)
	}
	job, err := db.Claim(ctx, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if job.Execution.ID != id || job.Execution.Type != "test" || !job.Execution.RunAt.Equal(when) || job.Execution.Timeout != time.Second {
		t.Fatalf("unexpected job: %+v", job)
	}
	var decoded map[string]string
	if err := json.Unmarshal(job.Execution.Payload, &decoded); err != nil || decoded["message"] != "hello" {
		t.Fatalf("payload = %s (%v)", job.Execution.Payload, err)
	}
	if _, err := db.Finish(ctx, job, errors.New("delivery failed")); err != nil {
		t.Fatal(err)
	}
	var status string
	var result string
	if err := pool.QueryRow(ctx, "SELECT status, last_error FROM jobs WHERE id=$1", id).Scan(&status, &result); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("status = %s", status)
	}
	if result != "delivery failed" {
		t.Fatalf("result = %s", result)
	}
	if _, err := db.Finish(ctx, job, nil); err == nil {
		t.Fatal("finished a terminal job again")
	}
	if _, err := db.Finish(ctx, multiline.Claim{Execution: multiline.Execution{ID: uuid.New()}, Token: uuid.New()}, nil); err == nil {
		t.Fatal("finished a missing job")
	}
	jobs, err := listJobs(ctx, db, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != id {
		t.Fatalf("jobs = %+v", jobs)
	}
	page, err := db.List(ctx, multiline.ListParams{Limit: 10, After: &multiline.JobCursor{CreatedAt: jobs[0].CreatedAt, ID: jobs[0].ID}})
	if err != nil || len(page.Jobs) != 0 {
		t.Fatalf("empty page = %v, %v", page, err)
	}
}

func TestTransactionOwnership(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	db := postgres.New(pool)
	for _, commit := range []bool{false, true} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
		bound := postgres.NewTx(tx)
		id, err := bound.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now().Add(-time.Second)})
		if err != nil {
			t.Fatal(err)
		}
		// Other connections cannot observe the uncommitted enqueue.
		jobs, err := listJobs(ctx, db, 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, job := range jobs {
			if job.ID == id {
				t.Fatal("uncommitted job visible")
			}
		}
		if commit {
			err = tx.Commit(ctx)
		} else {
			err = tx.Rollback(ctx)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := bound.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now()}); !errors.Is(err, pgx.ErrTxClosed) {
			t.Fatalf("closed transaction error = %v", err)
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE id=$1", id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if (count == 1) != commit {
			t.Fatalf("commit=%v, visible rows=%d", commit, count)
		}
	}
}

func TestEmptyPayload(t *testing.T) {
	ctx := t.Context()
	db := postgres.New(newTestPool(t))
	if _, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now(), Payload: []byte{}}); err != nil {
		t.Fatal(err)
	}
	jobs, err := listJobs(ctx, db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || len(jobs[0].Payload) != 0 {
		t.Fatalf("jobs = %+v", jobs)
	}
}

func TestValidationAndDatabaseErrors(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	db := postgres.New(pool)
	for _, job := range []multiline.EnqueueParams{
		{RunAt: time.Now()}, {Type: strings.Repeat("x", 21), RunAt: time.Now()},
		{Type: "test"}, {Type: "test", RunAt: time.Now(), Payload: []byte("{")},
		{Type: "test", RunAt: time.Now(), Timeout: -time.Second},
		{Type: "test", RunAt: time.Now(), MaxAttempts: 1001},
		{Type: "test", RunAt: time.Now(), RetryDelay: 25 * time.Hour},
		{Type: "test", RunAt: time.Now(), MaxRetryDelay: -time.Second},
		{Type: "test", RunAt: time.Now(), RetryDelay: time.Second, MaxRetryDelay: time.Millisecond},
		{Type: string([]byte{0xff}), RunAt: time.Now()},
		{Type: "test", RunAt: time.Now(), Timeout: time.Duration(2147483647)*time.Second + 1},
	} {
		if _, err := db.Enqueue(ctx, job); !errors.Is(err, multiline.ErrInvalid) {
			t.Fatalf("accepted invalid job: %+v", job)
		}
	}
	for _, params := range []multiline.ListParams{{Limit: -1}, {Limit: 1001}, {After: &multiline.JobCursor{}}} {
		if _, err := db.List(ctx, params); !errors.Is(err, multiline.ErrInvalid) {
			t.Fatalf("invalid page: %v", err)
		}
	}
	if _, err := db.Recover(ctx, 0); !errors.Is(err, multiline.ErrInvalid) {
		t.Fatalf("invalid recovery = %v", err)
	}
	if _, err := db.Prune(ctx, time.Time{}, 1); !errors.Is(err, multiline.ErrInvalid) {
		t.Fatalf("invalid retention time = %v", err)
	}
	if _, err := db.Prune(ctx, time.Now(), 0); !errors.Is(err, multiline.ErrInvalid) {
		t.Fatalf("invalid retention batch = %v", err)
	}
	pool.Close()
	if _, err := db.Claim(ctx, "test", time.Minute); err == nil {
		t.Fatal("closed pool claim succeeded")
	}
	if _, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now()}); err == nil {
		t.Fatal("closed pool enqueue succeeded")
	}
	if _, err := listJobs(ctx, db, 1); err == nil {
		t.Fatal("closed pool list succeeded")
	}
	if _, err := db.Finish(ctx, multiline.Claim{Execution: multiline.Execution{ID: uuid.New()}, Token: uuid.New()}, nil); err == nil {
		t.Fatal("closed pool finish succeeded")
	}
	if _, err := db.Recover(ctx, 1); err == nil {
		t.Fatal("closed pool recovery succeeded")
	}
	if _, err := db.Prune(ctx, time.Now(), 1); err == nil {
		t.Fatal("closed pool retention succeeded")
	}

}

func TestLeaseRecoveryAndFencing(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	db := postgres.New(pool)
	id, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now(), MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	first, err := db.Claim(ctx, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Renew(ctx, first, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	forged := first
	forged.Token = uuid.New()
	if err := db.Renew(ctx, forged, time.Minute); !errors.Is(err, multiline.ErrLeaseLost) {
		t.Fatalf("forged renewal: %v", err)
	}
	if _, err := db.Finish(ctx, forged, nil); !errors.Is(err, multiline.ErrLeaseLost) {
		t.Fatalf("forged completion: %v", err)
	}
	expire := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, "UPDATE jobs SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", id); err != nil {
			t.Fatal(err)
		}
	}
	expire()
	if err := db.Renew(ctx, first, time.Minute); !errors.Is(err, multiline.ErrLeaseLost) {
		t.Fatalf("expired renewal: %v", err)
	}
	if _, err := db.Finish(ctx, first, nil); !errors.Is(err, multiline.ErrLeaseLost) {
		t.Fatalf("expired completion: %v", err)
	}
	if _, err := db.Claim(ctx, "test", time.Minute); !errors.Is(err, multiline.ErrNoJob) {
		t.Fatalf("claim recovered implicitly: %v", err)
	}
	recovered, err := db.Recover(ctx, 1)
	if err != nil || recovered.Retried != 1 {
		t.Fatalf("recovery = %+v, %v", recovered, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET next_run_time=clock_timestamp() WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	second, err := db.Claim(ctx, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if second.Token == first.Token || second.Execution.Attempt != 2 || second.Execution.ID != id {
		t.Fatalf("reclaimed = %+v", second)
	}
	if _, err := db.Finish(ctx, first, nil); !errors.Is(err, multiline.ErrLeaseLost) {
		t.Fatalf("stale completion: %v", err)
	}
	if err := db.Renew(ctx, first, time.Minute); !errors.Is(err, multiline.ErrLeaseLost) {
		t.Fatalf("stale renewal: %v", err)
	}
	expire()
	if _, err := db.Claim(ctx, "test", time.Minute); !errors.Is(err, multiline.ErrNoJob) {
		t.Fatalf("final expired claim: %v", err)
	}
	jobs, err := listJobs(ctx, db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if recovered, err := db.Recover(ctx, 1); err != nil || recovered.Failed != 1 {
		t.Fatalf("final recovery = %+v, %v", recovered, err)
	}
	jobs, err = listJobs(ctx, db, 1)
	if err != nil {
		t.Fatal(err)
	}
	record := jobs[0]
	if record.Status != multiline.StatusFailed || record.Attempts != 2 || record.LeaseExpiresAt != nil || record.CompletedAt == nil || record.LastError == "" {
		t.Fatalf("exhausted = %+v", record)
	}
}

func TestRetryDelayAndSuccessfulRecovery(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	db := postgres.New(pool)
	id, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now(), MaxAttempts: 2, RetryDelay: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	first, err := db.Claim(ctx, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	if _, err := db.Finish(ctx, first, errors.New("temporary outage")); err != nil {
		t.Fatal(err)
	}
	jobs, err := listJobs(ctx, db, 1)
	if err != nil {
		t.Fatal(err)
	}
	pending := jobs[0]
	if pending.Status != multiline.StatusPending || pending.RunAt.Before(before) || pending.RunAt.After(time.Now().Add(time.Hour)) || pending.CompletedAt != nil || pending.LastError != "temporary outage" {
		t.Fatalf("retry = %+v", pending)
	}

	if _, err := pool.Exec(ctx, "UPDATE jobs SET next_run_time=clock_timestamp() WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
	second, err := db.Claim(ctx, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if second.Execution.Attempt != 2 {
		t.Fatalf("attempt = %d", second.Execution.Attempt)
	}
	if _, err := db.Finish(ctx, second, nil); err != nil {
		t.Fatal(err)
	}
	jobs, err = listJobs(ctx, db, 1)
	if err != nil {
		t.Fatal(err)
	}
	done := jobs[0]
	if done.Status != multiline.StatusCompleted || done.Attempts != 2 || done.CompletedAt == nil || done.LeaseExpiresAt != nil || done.LastError != "" {
		t.Fatalf("completed = %+v", done)
	}
}

func TestDatabaseConstraints(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	id, err := postgres.New(pool).Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	for _, assignment := range []string{
		"status='unknown'", "timeout_seconds=0", "status=NULL", "type=' '", "max_attempts=0", "attempts=-1",
		"retry_delay_ms=0", "retry_max_delay_ms=0", "attempts=1", "status='running'", "completed_at=now()", "claim_token=gen_random_uuid()",
	} {
		if _, err := pool.Exec(ctx, "UPDATE jobs SET "+assignment+" WHERE id=$1", id); err == nil {
			t.Fatalf("accepted invalid state: %s", assignment)
		}
	}
}

func TestSchedulerRenewsLeaseAcrossOriginalExpiry(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	pool := newTestPool(t)
	db := postgres.New(pool)
	if _, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "wait", RunAt: time.Now(), MaxAttempts: 2}); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	scheduler, err := multiline.NewScheduler(db, registered(t, map[string]multiline.Handler{"wait": func(ctx context.Context, _ multiline.Execution) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}), multiline.Options{LeaseDuration: 900 * time.Millisecond, PollInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- scheduler.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Error(err)
		}
	})
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Keep the handler active past its original lease; renewal must prevent a
	// second worker from claiming it, and completion must still be accepted.
	timer := time.NewTimer(1400 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := db.Claim(ctx, "wait", time.Minute); !errors.Is(err, multiline.ErrNoJob) {
		t.Fatalf("active job reclaimed: %v", err)
	}
	close(release)
	for {
		jobs, err := listJobs(ctx, db, 1)
		if err != nil {
			t.Fatal(err)
		}
		if jobs[0].Status == multiline.StatusCompleted {
			if jobs[0].Attempts != 1 {
				t.Fatalf("attempts = %d", jobs[0].Attempts)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestFreshMigrationRoundTrip(t *testing.T) {
	ctx := t.Context()
	pool := newSchemaPool(t, false)
	for _, name := range []string{"000001_jobs.up.sql", "000001_jobs.down.sql", "000001_jobs.up.sql"} {
		contents, err := os.ReadFile("../db/migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := postgres.New(pool).Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func TestPermanentErrorsSkipRetries(t *testing.T) {
	ctx := t.Context()
	db := postgres.New(newTestPool(t))
	if _, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now(), MaxAttempts: 10}); err != nil {
		t.Fatal(err)
	}
	claim, err := db.Claim(ctx, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	result, err := db.Finish(ctx, claim, fmt.Errorf("wrapped: %w", multiline.Permanent(errors.New("invalid address"))))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != multiline.StatusFailed || result.RetryAt != nil {
		t.Fatalf("permanent outcome = %+v", result)
	}
	jobs, err := listJobs(ctx, db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].Attempts != 1 || jobs[0].Status != multiline.StatusFailed {
		t.Fatalf("record = %+v", jobs[0])
	}
}

func TestBackoffJitterIsBoundedAndCapped(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	db := postgres.New(pool)
	id, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now(), MaxAttempts: 6, RetryDelay: time.Second, MaxRetryDelay: 4 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 6; attempt++ {
		claim, err := db.Claim(ctx, "test", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		var before time.Time
		if err := pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&before); err != nil {
			t.Fatal(err)
		}
		result, err := db.Finish(ctx, claim, errors.New("retry"))
		if err != nil {
			t.Fatal(err)
		}
		var after time.Time
		if err := pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&after); err != nil {
			t.Fatal(err)
		}
		if attempt == 6 {
			if result.Status != multiline.StatusFailed || result.RetryAt != nil {
				t.Fatalf("exhausted = %+v", result)
			}
			break
		}
		cap := min(time.Second*time.Duration(1<<(attempt-1)), 4*time.Second)
		if result.Status != multiline.StatusPending || result.RetryAt == nil || result.RetryAt.Before(before.Add(time.Millisecond)) || result.RetryAt.After(after.Add(cap)) {
			t.Fatalf("attempt %d retry not in jitter range: %+v, before=%v after=%v cap=%v", attempt, result, before, after, cap)
		}
		if _, err := pool.Exec(ctx, "UPDATE jobs SET next_run_time=clock_timestamp() WHERE id=$1", id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBoundedRecoveryAndRetention(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	db := postgres.New(pool)
	for range 3 {
		if _, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Claim(ctx, "test", time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET lease_expires_at=clock_timestamp()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	// Claim never mutates expired running state.
	if _, err := db.Claim(ctx, "test", time.Minute); !errors.Is(err, multiline.ErrNoJob) {
		t.Fatal(err)
	}
	for want := range 3 {
		result, err := db.Recover(ctx, 1)
		if err != nil || result.Failed != 1 || result.Retried != 0 {
			t.Fatalf("batch = %+v, %v", result, err)
		}
		var failed int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE status='failed'").Scan(&failed); err != nil {
			t.Fatal(err)
		}
		if failed != want+1 {
			t.Fatalf("failed = %d", failed)
		}
	}
	if _, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "pending", RunAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "running", RunAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Claim(ctx, "running", time.Minute); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now().Add(time.Second)
	for range 3 {
		if count, err := db.Prune(ctx, cutoff, 1); err != nil || count != 1 {
			t.Fatalf("prune = %d, %v", count, err)
		}
	}
	if count, err := db.Prune(ctx, cutoff, 1); err != nil || count != 0 {
		t.Fatalf("empty prune = %d, %v", count, err)
	}
	jobs, err := listJobs(ctx, db, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("retention removed live work: %+v", jobs)
	}
	for _, job := range jobs {
		if job.Status != multiline.StatusPending && job.Status != multiline.StatusRunning {
			t.Fatalf("unexpected retained status: %+v", job)
		}
	}
}

func TestCursorSurvivesReschedulingAndDeletedBoundary(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	db := postgres.New(pool)
	ids := make([]uuid.UUID, 5)
	for i := range ids {
		id, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now().Add(time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
	}
	// Exercise UUID tie-breaking, not just distinct timestamps.
	if _, err := pool.Exec(ctx, "UPDATE jobs SET created_at='2026-01-01T00:00:00Z'"); err != nil {
		t.Fatal(err)
	}
	first, err := db.List(ctx, multiline.ListParams{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Jobs) != 2 || first.Next == nil {
		t.Fatalf("first = %+v", first)
	}
	boundary := first.Next
	seen := map[uuid.UUID]bool{}
	for _, job := range first.Jobs {
		seen[job.ID] = true
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET next_run_time=clock_timestamp()-interval '1 day'"); err != nil {
		t.Fatal(err)
	}
	// A cursor is a value boundary, not a foreign key: it works after retention.
	if _, err := pool.Exec(ctx, "DELETE FROM jobs WHERE id=$1", boundary.ID); err != nil {
		t.Fatal(err)
	}
	for cursor := boundary; cursor != nil; {
		page, err := db.List(ctx, multiline.ListParams{Limit: 2, After: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, job := range page.Jobs {
			if seen[job.ID] {
				t.Fatalf("duplicate across pages: %s", job.ID)
			}
			seen[job.ID] = true
		}
		cursor = page.Next
	}
	if len(seen) != len(ids) {
		t.Fatalf("saw %d jobs, want %d", len(seen), len(ids))
	}
}

func TestCrashWorkerHelper(t *testing.T) {
	if os.Getenv("MULTILINE_CRASH_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(os.Getenv("MULTILINE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = os.Getenv("MULTILINE_CRASH_SCHEMA")
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	scheduler, err := multiline.NewScheduler(postgres.New(pool), registered(t, map[string]multiline.Handler{"crash": func(context.Context, multiline.Execution) error {
		os.Exit(23) // Simulate process death after the claim commits, before completion.
		return nil
	}}), multiline.Options{LeaseDuration: 90 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Fatal(scheduler.Run(ctx))
}

func TestRecoveryAfterWorkerProcessDeath(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	pool := newTestPool(t)
	db := postgres.New(pool)
	id, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "crash", RunAt: time.Now(), MaxAttempts: 2, RetryDelay: time.Millisecond, MaxRetryDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCrashWorkerHelper$")
	child.Env = append(os.Environ(), "MULTILINE_CRASH_HELPER=1", "MULTILINE_CRASH_SCHEMA="+pool.Config().ConnConfig.RuntimeParams["search_path"])
	output, err := child.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 23 {
		t.Fatalf("worker did not crash at the execution boundary: %v\n%s", err, output)
	}
	jobs, err := listJobs(ctx, db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].Status != multiline.StatusRunning || jobs[0].LeaseExpiresAt == nil {
		t.Fatalf("abandoned = %+v", jobs[0])
	}
	wait := time.NewTimer(max(time.Until(*jobs[0].LeaseExpiresAt)+20*time.Millisecond, time.Millisecond))
	defer wait.Stop()
	select {
	case <-wait.C:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	recovered, err := db.Recover(ctx, 1)
	if err != nil || recovered.Retried != 1 {
		t.Fatalf("crash recovery = %+v, %v", recovered, err)
	}
	for {
		claim, err := db.Claim(ctx, "crash", time.Minute)
		if errors.Is(err, multiline.ErrNoJob) {
			select {
			case <-time.After(time.Millisecond):
				continue
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		if claim.Execution.ID != id || claim.Execution.Attempt != 2 {
			t.Fatalf("replacement = %+v", claim)
		}
		if _, err := db.Finish(ctx, claim, nil); err != nil {
			t.Fatal(err)
		}
		break
	}
}

func TestDatabaseFailureCancelsActiveHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	pool := newTestPool(t)
	db := postgres.New(pool)
	if _, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "wait", RunAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	started, canceled := make(chan struct{}), make(chan struct{})
	scheduler, err := multiline.NewScheduler(db, registered(t, map[string]multiline.Handler{"wait": func(ctx context.Context, _ multiline.Execution) error {
		close(started)
		<-ctx.Done()
		close(canceled)
		return ctx.Err()
	}}), multiline.Options{RecoveryInterval: 10 * time.Millisecond, ShutdownGracePeriod: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- scheduler.Run(ctx) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Interrupt database availability independently of handler execution.
	pool.Close()
	select {
	case err := <-done:
		if err == nil || ctx.Err() != nil {
			t.Fatalf("Run = %v, context = %v", err, ctx.Err())
		}
	case <-ctx.Done():
		t.Fatal("database failure did not stop Run")
	}
	select {
	case <-canceled:
	default:
		t.Fatal("database failure did not cancel handler")
	}
}

func TestRetentionCutoffIsStrict(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	db := postgres.New(pool)
	cutoff := time.Now().UTC().Truncate(time.Microsecond)
	for _, completedAt := range []time.Time{cutoff.Add(-time.Second), cutoff, cutoff.Add(time.Second)} {
		id, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		claim, err := db.Claim(ctx, "test", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Finish(ctx, claim, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE jobs SET completed_at=$2 WHERE id=$1", id, completedAt); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := db.Prune(ctx, cutoff, 100); err != nil || count != 1 {
		t.Fatalf("pruned = %d, %v", count, err)
	}
	jobs, err := listJobs(ctx, db, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("retained %d records", len(jobs))
	}
	for _, job := range jobs {
		if job.CompletedAt == nil || job.CompletedAt.Before(cutoff) {
			t.Fatalf("retained = %+v", job)
		}
	}
}

func TestConcurrentRecovery(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	db := postgres.New(pool)
	for i := range 20 {
		if _, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now(), MaxAttempts: 1 + int32(i%2)}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Claim(ctx, "test", time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET lease_expires_at=clock_timestamp()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	results := make(chan multiline.RecoveryResult, 24)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for {
				result, err := db.Recover(ctx, 3)
				if err != nil {
					t.Error(err)
					return
				}
				if result.Retried+result.Failed == 0 {
					return
				}
				results <- result
			}
		})
	}
	wg.Wait()
	close(results)
	var total multiline.RecoveryResult
	for result := range results {
		total.Retried += result.Retried
		total.Failed += result.Failed
	}
	if total.Retried != 10 || total.Failed != 10 {
		t.Fatalf("recovered more than once or missed rows: %+v", total)
	}
}

func TestPersistenceSizeLimits(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	db := postgres.New(pool)
	boundary := []byte(`"` + strings.Repeat("x", multiline.MaxPayloadBytes-2) + `"`)
	id, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now(), Payload: boundary, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := db.Claim(ctx, "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(claim.Execution.Payload) != multiline.MaxPayloadBytes {
		t.Fatalf("payload length = %d", len(claim.Execution.Payload))
	}
	// Persist only bounded sanitized text; retry classification still uses the
	// original wrapped error. An oversized permanent error must not be retried.
	huge := fmt.Errorf("wrapped: %w", multiline.Permanent(errors.New("bad\x00UTF-8\xff"+strings.Repeat("€", multiline.MaxErrorBytes))))
	result, err := db.Finish(ctx, claim, huge)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != multiline.StatusFailed {
		t.Fatalf("classification changed: %+v", result)
	}
	jobs, err := listJobs(ctx, db, 1)
	if err != nil {
		t.Fatal(err)
	}
	message := jobs[0].LastError
	if len(message) > multiline.MaxErrorBytes || !utf8.ValidString(message) || strings.ContainsRune(message, 0) || !strings.HasSuffix(message, "... [truncated]") {
		t.Fatalf("stored message violates bounds: %q", message)
	}
	tooLarge := []byte(`"` + strings.Repeat("x", multiline.MaxPayloadBytes-1) + `"`)
	if _, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: "test", RunAt: time.Now(), Payload: tooLarge}); !errors.Is(err, multiline.ErrInvalid) {
		t.Fatalf("oversized enqueue = %v", err)
	}
	// Clients bypassing the Go adapter still cannot grow these columns unboundedly.
	if _, err := pool.Exec(ctx, "UPDATE jobs SET payload=$2 WHERE id=$1", id, tooLarge); err == nil {
		t.Fatal("database accepted oversized payload")
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET last_error=$2 WHERE id=$1", id, strings.Repeat("x", multiline.MaxErrorBytes+1)); err == nil {
		t.Fatal("database accepted oversized error")
	}
	exact := strings.Repeat("x", multiline.MaxErrorBytes)
	if _, err := pool.Exec(ctx, "UPDATE jobs SET last_error=$2 WHERE id=$1", id, exact); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresRepresentationLimitIsValidationError(t *testing.T) {
	ctx := t.Context()
	db := postgres.New(newTestPool(t))
	// jsonb inserts a space after the colon; its text is one byte larger than the
	// submitted JSON. Adapter-specific storage validation preserves the PG cause.
	payload := []byte(`{"a":"` + strings.Repeat("x", multiline.MaxPayloadBytes-8) + `"}`)
	params := multiline.EnqueueParams{Type: "test", RunAt: time.Now(), Payload: payload}
	if _, err := params.Normalize(); err != nil {
		t.Fatalf("submitted JSON should fit: %v", err)
	}
	_, err := db.Enqueue(ctx, params)
	var detail *multiline.ValidationError
	var cause *pgconn.PgError
	if !errors.Is(err, multiline.ErrInvalid) || !errors.As(err, &detail) || detail.Field != "payload" || !errors.As(err, &cause) || cause.ConstraintName != "jobs_payload_size" {
		t.Fatalf("representation size error = %v", err)
	}
}

func TestClaimFiltersJobType(t *testing.T) {
	ctx := t.Context()
	pool := newTestPool(t)
	db := postgres.New(pool)
	ids := map[string]uuid.UUID{}
	for i, jobType := range []string{"email", "report", "unknown"} {
		id, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: jobType, RunAt: time.Now().Add(-time.Duration(3-i) * time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
		ids[jobType] = id
	}
	// An older email must not occupy a report worker or another type's lease.
	report, err := db.Claim(ctx, "report", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if report.Execution.ID != ids["report"] || report.Execution.Type != "report" {
		t.Fatalf("wrong pool claimed job: %+v", report)
	}
	if _, err := db.Claim(ctx, "report", time.Minute); !errors.Is(err, multiline.ErrNoJob) {
		t.Fatalf("report worker claimed another type: %v", err)
	}
	if _, err := db.Claim(ctx, "", time.Minute); !errors.Is(err, multiline.ErrInvalid) {
		t.Fatalf("empty type allowed wildcard: %v", err)
	}
	records, err := listJobs(ctx, db, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.Type != "report" && (record.Status != multiline.StatusPending || record.Attempts != 0 || record.LeaseExpiresAt != nil) {
			t.Fatalf("unselected job was claimed: %+v", record)
		}
	}
	email, err := db.Claim(ctx, "email", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if email.Execution.ID != ids["email"] {
		t.Fatal("email claim did not use its own backlog")
	}
}

func TestDedicatedPoolsWithPostgresBacklog(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	pool := newTestPool(t)
	db := postgres.New(pool)
	for _, jobType := range []string{"slow", "fast", "unknown"} {
		for range 5 {
			if _, err := db.Enqueue(ctx, multiline.EnqueueParams{Type: jobType, RunAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
		}
	}
	started := make(chan string, 15)
	handler := func(ctx context.Context, execution multiline.Execution) error {
		started <- execution.Type
		<-ctx.Done()
		return ctx.Err()
	}
	registry := multiline.NewWorkerRegistry()
	if err := registry.Register("slow", handler, multiline.WorkerOptions{Concurrency: 2}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("fast", handler); err != nil {
		t.Fatal(err)
	}
	scheduler, err := multiline.NewScheduler(db, registry, multiline.Options{PollInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- scheduler.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Error(err)
		}
	})
	counts := map[string]int{}
	for range 3 {
		select {
		case jobType := <-started:
			counts[jobType]++
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if counts["slow"] != 2 || counts["fast"] != 1 {
		t.Fatalf("active pools = %v", counts)
	}
	records, err := listJobs(ctx, db, 100)
	if err != nil {
		t.Fatal(err)
	}
	running, pending := map[string]int{}, map[string]int{}
	for _, record := range records {
		if record.Status == multiline.StatusRunning {
			running[record.Type]++
		} else if record.Status == multiline.StatusPending {
			pending[record.Type]++
		}
	}
	if running["slow"] != 2 || running["fast"] != 1 || pending["slow"] != 3 || pending["fast"] != 4 || pending["unknown"] != 5 {
		t.Fatalf("capacity/routing changed queue unexpectedly: running=%v pending=%v", running, pending)
	}
}
