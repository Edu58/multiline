# multiline

A Go library for persistent, one-shot background jobs backed by PostgreSQL.
Applications own connections, configuration, handlers, and process lifecycle.
Workers claim jobs atomically, renew leases, and reject stale completion writes.
Retries, recovery, inspection, and retention have explicit bounded operations.

```sh
go get github.com/Edu58/multiline
```

Apply `db/migrations/000001_jobs.up.sql` to an empty database with a migration
tool. This is a **fresh schema baseline**; earlier development migrations were
removed. It is not an upgrade script for the old schema. No existing application
database is reset or migrated automatically. Rollback drops the jobs table.

```go
pool, err := pgxpool.New(ctx, dsn)
if err != nil {
    return err
}
defer pool.Close()
if err := pool.Ping(ctx); err != nil {
    return err
}
db := postgres.New(pool)

_, err = db.Enqueue(ctx, multiline.EnqueueParams{
    Type: "email",
    RunAt: time.Now().UTC(),
    Payload: json.RawMessage(`{"recipient":"reader@example.com"}`),
    MaxAttempts: 3,
    RetryDelay: time.Second,
    MaxRetryDelay: time.Minute,
})
if err != nil {
    return err
}

registry := multiline.NewWorkerRegistry()
if err := registry.Register("email", func(ctx context.Context, execution multiline.Execution) error {
    err := sendEmail(ctx, execution.ID.String(), execution.Payload)
    // Return multiline.Permanent(err) for errors that retry cannot fix.
    return err
}, multiline.WorkerOptions{Concurrency: 8}); err != nil {
    return err
}

scheduler, err := multiline.NewScheduler(db, registry, multiline.Options{
    ShutdownGracePeriod: 30 * time.Second,
})
if err != nil {
    return err
}
return scheduler.Run(ctx)
```

The snippet assumes application-owned `ctx`, `dsn`, and `sendEmail`; import
`context`, `encoding/json`, `time`, the root package, its `postgres` package, and
`github.com/jackc/pgx/v5/pgxpool`. See the [compiling lifecycle example](example_test.go).

## Handler registration and dispatch

Create a `WorkerRegistry` during application startup and call
`Register(jobType, handler, optionalWorkerOptions)` from your application modules. Job types are
case-sensitive keys that map directly to `Handler` functions. Register validates
names and rejects nil handlers with `ErrInvalid`; duplicates return
`ErrDuplicateHandler` without replacing the existing registration. A registry's
zero value is usable, and registration/scheduler construction are safe concurrently.
Do not copy a registry after first use.

`NewScheduler` takes a nonempty registry and snapshots its registrations. Later
registrations affect only subsequently constructed schedulers, including when an
existing scheduler is restarted. Handler function state is still shared, so each
handler must be safe for concurrent use. A private dispatcher selects the handler
by `Execution.Type`. The attempt runner retains timeouts, panic containment, lease renewal, and result persistence.

### Dedicated handler pools

Each registration gets **one worker by default**, including explicit zero-valued
options. Set a positive count to reserve more workers for that handler:

```go
if err := registry.Register("email", sendEmail,
    multiline.WorkerOptions{Concurrency: 8}); err != nil {
    return err
}
if err := registry.Register("report", generateReport,
    multiline.WorkerOptions{Concurrency: 2}); err != nil {
    return err
}
// Omitting WorkerOptions creates one worker for this handler.
```

These pools claim only their own job types and execute immediately; jobs do not
hold leases waiting for a handler slot. A busy email pool cannot consume report
capacity. Total worker count is the sum of registered counts. Limits apply per
scheduler instance: three processes with eight email workers each permit up to
24 concurrent email attempts. They are not cluster-wide limits. Negative counts
and multiple WorkerOptions arguments are invalid. Counts are snapshotted with
handlers at scheduler construction.

Unregistered job types stay pending until a scheduler with that registration is
started. All schedulers may share the database while registering different types.
The private dispatcher still treats an unexpected type as a permanent error,
but a conforming Repository never returns another type to a worker.

**API changes:** `NewScheduler` takes a `*WorkerRegistry` instead of a handler map.
Move old `Options.Concurrency` values into each registration's
`WorkerOptions.Concurrency`; there is no scheduler-wide worker count.
Custom repositories must now implement `Claim(ctx, jobType, leaseDuration)` and
filter atomically by that type. Empty types are invalid, never a wildcard.
Register each key/value pair explicitly and handle errors before construction.

## Types and transaction ownership

`EnqueueParams` describes a new job. `Execution` supplies handler data and the
attempt number. `Claim` contains ownership credentials, which handlers never
receive. `JobRecord` exposes persisted status, timestamps, policy, and last error.
The root package depends on neither a database driver nor a logging framework.

`postgres.New(pool)` returns a concurrent `Store` implementing `Repository`.
It performs no I/O and never closes the pool. The application verifies the pool
and closes it after all schedulers stop. Generated sqlc code is internal.

`postgres.NewTx(tx)` returns a distinct `TxStore` exposing only `Enqueue` and
`List`. It **cannot satisfy Repository** or be passed to a scheduler. Perform
application writes and enqueue within that transaction, then commit or roll back
in the application. A transaction-bound store must not be used concurrently.
See the [transaction example](postgres/example_test.go).

## Shared validation and storage limits

`EnqueueParams.Normalize()` is the common domain entry point for every adapter.
It validates fields, supplies defaults, rounds durations, generates a missing ID,
converts RunAt to UTC, and returns an independent payload copy. It does not
modify caller input. Calling it again preserves the generated ID and values.
PostgreSQL enqueue calls it automatically; applications may call it earlier for
validation or to obtain an ID before persistence. Reuse that normalized value
when retrying an uncertain enqueue with the same deduplication key.

Submitted payload JSON is limited to **256 KiB** (`MaxPayloadBytes`) and must be
valid UTF-8 JSON. Oversized payloads return `ErrInvalid` with field `payload`.
PostgreSQL also bounds its normalized jsonb text to 256 KiB; its spacing and
number representation may expand submitted JSON, so input near the limit can
still fail storage validation. That failure also matches `ErrInvalid` and retains
the PostgreSQL cause. Use object storage and a reference for larger data.

Persisted error messages are limited to **4 KiB** (`MaxErrorBytes`). The shared
`ErrorMessage(err)` helper replaces NUL/invalid UTF-8, truncates at a rune boundary,
and includes `... [truncated]` within the limit. Nil becomes an empty string.
The original error still controls permanent/retry classification and is passed
to logs/observers. Limits on stored messages do not limit application logging.
Database constraints enforce payload and error byte limits for other SQL clients.
These constraints are included in the fresh schema baseline.

## Attempts, leases, and retry policy

Claims select only due pending work matching the requesting worker's job type
and atomically increment the attempt count.
A fresh token fences renewal and completion. Leases default to 30 seconds and
renew every third of that duration while the handler runs, including during
shutdown grace. Expired leases cannot be resurrected or completed. Database
clock time decides eligibility; worker clock skew does not control ownership.

`MaxAttempts` defaults to **one**, including the initial attempt. Set it above one
to opt into retries. Failures use capped exponential backoff with full jitter:

```text
cap = min(MaxRetryDelay, RetryDelay * 2^(attempt - 1))
delay = a random whole number of milliseconds from 1 through cap
```

`RetryDelay` defaults to one second; `MaxRetryDelay` defaults to the larger of
one hour and RetryDelay. Both are limited to 24 hours and round up to milliseconds.
Timeout defaults to 15 seconds and rounds up to seconds. Handler errors, panics,
timeouts, and forced cancellation consume attempts. Success clears the last
error and marks the job completed. `Permanent(err)` skips retries and preserves
the underlying error; wrapped permanent errors remain classifiable. Jobs whose
types have no registered pool remain pending rather than consuming attempts.

`Finish` returns the actual persisted `FinishResult`, including retry time.
A handler returning nil does not imply successful result persistence.

**Leases fence database state, not external effects.** A paused worker can resume
after another worker takes over and still issue an external request. Use the
stable job ID with an idempotent destination or an application deduplication
transaction. Attempt numbers identify retries, not logical side effects.
Exactly-once effects and recurring schedules are not provided.

## Separate bounded recovery

`Recover(ctx, limit)` processes at most 1..1000 expired running attempts. It
reschedules jobs with attempts remaining using the same backoff/jitter policy,
and marks exhausted work failed. It returns retry/failure counts and never
claims a job. `Claim` never performs recovery as a hidden side effect.

Each scheduler runs a separate recovery loop at startup and every
`RecoveryInterval` (default one second), processing one `RecoveryBatchSize`
(default 100) batch per iteration. Multiple processes may recover concurrently
using SKIP LOCKED. Independent maintenance processes can also call Recover.
Recovery and then retry eligibility are required before an abandoned job can run
again. Database failures during recovery stop Run and cancel active handlers.

## Shutdown and failures

`NewScheduler` validates and snapshots registration without starting goroutines.
`Run` starts the configured dedicated pools (one worker per handler by default),
polls empty pools every second, and owns and joins worker, recovery, and renewal
goroutines.

Canceling Run's context immediately stops new claim/recovery requests. In-flight
claims may still complete. Active handlers retain context values and their own
job timeouts during `ShutdownGracePeriod`; after that period they are canceled.
Zero grace cancels immediately. Infrastructure failures bypass grace. Run waits
for handler exits and result writes, then returns the original cancellation or
infrastructure error. A handler ignoring cancellation can delay return indefinitely.

Renewal stops after handler exit. Result writes use a separate bounded context
(default five seconds). An expired lease rejects that result. Choose leases
comfortably longer than completion time and database latency; tiny leases are
for tests. Size the connection pool for the sum of configured worker counts,
handlers' database usage, renewal, and recovery. Each idle worker polls its own
type, so more workers also increase idle query traffic. Recovery remains one
queue-wide loop per scheduler, independent of which types it registers.

## Cursor inspection and retention

```go
page, err := db.List(ctx, multiline.ListParams{Limit: 100})
// Use page.Jobs, then pass page.Next as After while it is non-nil:
next, err := db.List(ctx, multiline.ListParams{Limit: 100, After: page.Next})
```

Only request another page if Next is non-nil. Cursors use immutable
`(created_at, id)`, so retry scheduling cannot move records across the boundary.
They work even after the boundary row is deleted. Pages are not one database
snapshot: concurrent writes can affect which records are observed. Treat cursors
as opaque values; applications may encode them for transport.

`Prune(ctx, before, limit)` deletes at most 1..1000 terminal jobs completed
strictly before the cutoff and returns the count. It never removes pending or
running work. Applications choose retention and schedule these calls; repeat
bounded calls to clear history without an unbounded deletion transaction.

## Errors and observability

Use `errors.Is` for `ErrInvalid`, `ErrDuplicateJob`, `ErrLeaseLost`, `ErrNoJob`,
`ErrDuplicateHandler`, and `ErrRunning`, and `errors.As` for `*ValidationError` field details.
`IsPermanent` classifies non-retryable handler failures. PostgreSQL causes remain
wrapped for diagnostics.

`Options.Observer` receives typed operational events for claims, renewal,
lease loss, persisted completion/failure/retry, persistence failures, and recovery
batches. Events include operation duration, IDs/attempts where applicable, errors,
retry time, and recovery counts. They contain no payloads or claim tokens.
Observers are synchronous and concurrent: return promptly, synchronize shared
state, and avoid high-cardinality metric labels such as IDs. Observer panics are
contained and optionally logged; callbacks are best-effort telemetry, not audit
storage. Empty claims emit no event; zero-count recovery batches do.

Handler middleware through `Chain` measures execution separately from persistence
and supports application-owned tracing. See the [instrumentation examples](middleware_example_test.go).
An optional `slog.Logger` reports failed handler attempts and observer panics.

## Development

```sh
make check
make race
```

Integration tests require a disposable PostgreSQL database and permission to
create/drop isolated schemas; they apply the fresh migration themselves:

```sh
MULTILINE_TEST_DSN='postgresql://postgres:postgres@localhost:5432/multiline_test?sslmode=disable' make integration
```

Edit named SQL in `db/queries/` and regenerate with `make sqlc` using sqlc
**v1.31.1**. CI verifies generation, formatting, vet, race tests, fresh migrations,
and integration tests. The suite includes worker-process death and interrupted
database availability. See [design notes](docs/library-design.md) for rationale
and remaining deployment qualification work.
