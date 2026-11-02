package multiline

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// EnqueueParams describes a new one-shot job. ID zero generates an ID. RunAt may
// be in the past. Timeout zero defaults to 15 seconds, MaxAttempts to one, and
// RetryDelay to one second and MaxRetryDelay to max(one hour, RetryDelay).
// Failed attempts use capped exponential backoff with full millisecond jitter.
// Retries are opt-in and require idempotent handlers. Normalize provides shared
// validation/defaults before persistence; adapters call it automatically.
type EnqueueParams struct {
	ID            uuid.UUID
	Type          string
	Payload       json.RawMessage
	RunAt         time.Time
	Timeout       time.Duration
	MaxAttempts   int32
	RetryDelay    time.Duration
	MaxRetryDelay time.Duration
}

// Execution is the data delivered to a handler. Attempt starts at one. ID stays
// the same across attempts and is suitable as an external idempotency key.
type Execution struct {
	ID      uuid.UUID
	Type    string
	Payload json.RawMessage
	RunAt   time.Time
	Timeout time.Duration
	Attempt int32
}

// Claim identifies one leased attempt. Token fences database writes from older
// workers. Handlers receive only Execution, not ownership credentials.
type Claim struct {
	Execution Execution
	Token     uuid.UUID
}

// Status is the persisted lifecycle state of a job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// JobRecord is a persisted snapshot suitable for inspection and administration.
// Nil timestamps indicate events that have not happened. LastError retains the
// most recent failure while awaiting a retry; successful completion clears it.
type JobRecord struct {
	ID             uuid.UUID
	Type           string
	Payload        json.RawMessage
	RunAt          time.Time
	Timeout        time.Duration
	Status         Status
	Attempts       int32
	MaxAttempts    int32
	RetryDelay     time.Duration
	MaxRetryDelay  time.Duration
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	StartedAt      *time.Time
	CompletedAt    *time.Time
	LeaseExpiresAt *time.Time
}

// FinishResult reports the persisted outcome, not merely the handler result.
// RetryAt is set only when a failed attempt was rescheduled.
type FinishResult struct {
	Status  Status
	RetryAt *time.Time
}

// RecoveryResult counts expired attempts rescheduled or terminalized in one batch.
type RecoveryResult struct{ Retried, Failed int64 }

// JobCursor identifies an immutable creation-order boundary, independent of
// retry rescheduling. Treat it as opaque and use the Next value returned by List.
type JobCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// ListParams selects a bounded page. Limit zero defaults to 100, maximum 1000.
type ListParams struct {
	Limit int32
	After *JobCursor
}

// JobPage contains records and a cursor only when more records were observed.
// Concurrent insertion/deletion means pages do not represent a single snapshot.
type JobPage struct {
	Jobs []JobRecord
	Next *JobCursor
}

// Repository persists leased jobs. Claim acquires only due pending work; Recover
// separately reschedules or terminalizes expired leases in bounded batches.
// Renew and Finish reject expired/replaced tokens with ErrLeaseLost. Finish
// classifies permanent errors and applies the persisted retry policy. Every
// method must honor contexts and support concurrent use.
type Repository interface {
	Claim(context.Context, time.Duration) (Claim, error)
	Renew(context.Context, Claim, time.Duration) error
	Finish(context.Context, Claim, error) (FinishResult, error)
	Recover(context.Context, int32) (RecoveryResult, error)
}

// Handler executes one attempt. It must support concurrent use and cancellation.
// External effects must be idempotent: leases cannot prevent duplicate effects.
type Handler func(context.Context, Execution) error
