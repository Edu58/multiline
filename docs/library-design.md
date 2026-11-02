# Building a production Go library

The root package owns domain contracts and execution lifecycle. PostgreSQL owns
persistent queue state. Applications own connections, configuration, handlers,
shutdown, retention policy, and instrumentation. The fresh migration contains
only the current model; development migration history and unused legacy columns
are removed rather than carried into a new library release.

## Make invalid ownership harder to express

EnqueueParams, Execution, Claim, and JobRecord separate scheduling intent,
handler data, lease credentials, and persisted inspection. Generated types live
inside postgres/internal/dbgen. The adapter translates rows and database errors
without exposing generated objects to applications.

A pool-backed Store implements Repository. A transaction-bound TxStore exposes
only Enqueue and List, so it cannot be passed to the scheduler by accident.
Both reuse private enqueue/inspection code; transaction ownership stays with the
caller. A test checks their method sets. This replaces a documentation-only rule
with a restriction enforced by the type system.

## Make registration explicit and dispatch immutable

WorkerRegistry maps job types directly to Handler functions and per-handler
WorkerOptions. Register validates keys, nil handlers, duplicate names, and worker
counts, and supports safe concurrent registration. A scheduler copies registration
and counts under a read lock during construction.
It never observes later additions, so startup composition across application modules
does not introduce dynamic routing changes for running or restarted schedulers.
Snapshots copy function references; they do not clone mutable handler state.

A private dispatcher resolves Execution.Type and invokes the selected handler.
Unexpected types are permanent errors as a defensive boundary. It owns no lifecycle policy: the attempt
runner still controls timeout/panic containment, ownership renewal, and completion.
This makes the registry public for application composition without exposing a
second public execution API or adding job-type-to-worker-name indirection.

## Reserve capacity before claiming work

Each registration creates a dedicated pool, defaulting to one worker. WorkerOptions
can increase its concurrency independently of other types. The scheduler-wide
Concurrency option is removed to avoid two competing meanings for the same limit.
Total concurrency is the sum of registration counts within one scheduler instance;
other instances have their own limits.

A pool's claim query filters by job type before locking a due row. An index on
(type, next_run_time, id) for pending jobs supports this access pattern. Jobs
remain pending until a worker has capacity, rather than acquiring leases while
waiting behind an in-memory semaphore. A full pool cannot consume another type's
workers. Unregistered jobs remain pending and can be served by another scheduler
with that registration. Custom repositories must preserve this routing guarantee;
the scheduler stops on a mismatched claim instead of borrowing another pool's
capacity. Recovery remains queue-wide and separately bounded.

## Centralize input policy before persistence

EnqueueParams.Normalize owns validation, defaults, precision, ID generation, and
payload ownership. The PostgreSQL adapter invokes it and only maps normalized
values into generated parameters. Another adapter can reuse the same rules
without importing PostgreSQL or copying validation logic. The method returns a
copy, generates an ID only after validation, and preserves it on repeated calls.
Handler registration uses the same job-type validator.

The domain limits submitted payloads to 256 KiB; PostgreSQL also constrains its
normalized jsonb text representation. Errors from that extra storage check remain
typed input errors while preserving the database cause. ErrorMessage creates a
safe 4 KiB persistence string without changing the original error's identity or
retry classification. It scans only the retained prefix, replaces invalid UTF-8
and NUL, and truncates on rune boundaries with an explicit marker. SQL constraints
protect the same columns from clients bypassing the adapter.

## Separate scheduling coordination from an attempt

Scheduler coordinates intake, recovery, concurrency, and shutdown grace. A private
attemptRunner owns handler invocation, timeout/panic containment, lease renewal,
and result persistence. It depends only on Renew and Finish, so it cannot claim
or recover work. Its immutable configuration is shared by workers.

An attempt joins its heartbeat before writing a result with an independent,
bounded context. Renewal failure cancels execution; uncertain or lost ownership
cannot produce a stale completion. Extracting this responsibility keeps the
scheduler's shutdown flow reviewable without adding a public abstraction. Tests
use a smaller repository fake to check renewal is joined and context values
survive cancellation during completion.

## Keep claim and recovery costs separate

Claim atomically selects due pending work, increments attempts, and assigns a
new token/lease with SKIP LOCKED. Renew and Finish require both the token and an
unexpired database-clock lease. Expired credentials cannot resurrect themselves.

Recover is a separate bounded operation, not a claim side effect. A scheduler
owns one recovery loop and processes one configured batch each interval.
Independent maintenance processes can call the same operation. Recovery counts
are observable, and query cost can be tuned separately from claim polling.
Expired jobs with attempts remaining return to pending with backoff; exhausted
jobs become terminal. Indexes serve pending claims, expired leases, immutable
pagination, and terminal retention independently.

## Retry policy is persisted policy

MaxAttempts includes the initial attempt and defaults to one. RetryDelay is the
initial exponential cap; MaxRetryDelay bounds it. PostgreSQL applies full jitter
in integer milliseconds during both completion and recovery, so policy is atomic
with state transitions and shared by all workers. The exponent is bounded before
calculation; all allowed policies reach their maximum well before that bound.

Permanent wraps an error without losing its cause and bypasses retries. Unknown
handler registration is a permanent failure. Other handler errors, timeouts,
panics, and forced cancellation consume attempts under the persisted policy.
Finish returns persisted outcome and retry time instead of requiring the
scheduler to guess whether the job became pending or terminal.

Leases still cannot guarantee exactly-once effects. A paused process can resume
and issue an external request after replacement. Use the stable job ID with an
idempotent destination. Test interruption before and after effects; attempts are
not new logical operations.

## Use stable pagination and bounded cleanup

Pagination follows created_at and ID, not next_run_time, which changes during
retry. A cursor is a value boundary and survives deletion of its row. Fetching
one extra record detects whether another page was observed. Concurrent inserts
and deletes still mean this is not a multi-call snapshot; transactions can also
commit rows whose creation boundary precedes an earlier page.

Prune deletes bounded batches of terminal jobs older than an explicit completion
cutoff. The application decides retention. No automatic background deletion or
unbounded delete is hidden in startup or claims.

## Distinguish intake cancellation from execution cancellation

Run uses separate contexts for claim/recovery and active handlers. Parent
cancellation stops intake immediately, then gives handlers a configurable grace
period before canceling them. Values are preserved; job-specific timeouts still
apply. Infrastructure failures cancel both contexts immediately.

Run joins all goroutines before returning. Renewal continues while a draining
handler exits, then stops before bounded result persistence. A lease expiring
during persistence rejects that write. Handlers must respect cancellation; Go
cannot forcibly kill an uncooperative goroutine. A grace timeout bounds when
cancellation is requested, not when every possible handler has stopped.

## Observe execution and persistence separately

Handler middleware measures attempts and propagates tracing context. Operational
observers report claims, renewals, lost ownership, persistence failures, actual
terminal/retry outcomes, and recovery counts. A successful handler does not imply
its result was saved. Callbacks omit payloads and ownership tokens, are concurrent
and synchronous, and must return quickly. Panics are contained so instrumentation
does not turn successful work into a failed job. These events are not durable
notifications; use application-owned durable mechanisms if delivery matters.

## Verify failure boundaries

Unit tests cover registry validation, duplicate/snapshot behavior, concurrent
registration, independent pool limits and the one-worker default, shared normalization and ownership, UTF-8/size boundaries,
concurrency limits, graceful drain and cancellation, renewal
failure, permanent errors, observer outcomes/panics, and middleware composition.
PostgreSQL tests cover atomic type-filtered claims, expired/stale/forged tokens, retry bounds,
recovery batches, cursor ties/deleted boundaries, retention, transaction visibility,
constraints, payload/error persistence limits, and the fresh migration up/down cycle. A subprocess dies inside a
handler, leaving its committed claim for recovery. Another test interrupts pool
availability and checks that infrastructure failure cancels active execution.

Before release, load-test realistic queue sizes and query plans, test network
partitions and database failover, and run against every supported PostgreSQL
version. Validate payload retention/access policies, establish queue-age and
lease-loss alerts, and publish a versioned API policy. Integration tests are
repeatable evidence, not a substitute for deployment-specific qualification.
