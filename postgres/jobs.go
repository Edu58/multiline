package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Edu58/multiline"
	"github.com/Edu58/multiline/postgres/internal/dbgen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ multiline.Repository = (*Store)(nil)

func invalid(field, reason string) error {
	return &multiline.ValidationError{Field: field, Reason: reason}
}
func validateLease(lease time.Duration) error {
	if lease < 30*time.Millisecond || lease > 24*time.Hour {
		return invalid("lease_duration", "must be between 30 milliseconds and 24 hours")
	}
	return nil
}
func tokenValue(token uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: token, Valid: true} }

// Claim acquires one due pending job of exactly jobType. Expired running jobs are handled only by Recover.
func (s *Store) Claim(ctx context.Context, jobType string, lease time.Duration) (multiline.Claim, error) {
	if err := multiline.ValidateJobType(jobType); err != nil {
		return multiline.Claim{}, err
	}
	if err := validateLease(lease); err != nil {
		return multiline.Claim{}, err
	}
	token := uuid.New()
	row, err := s.queries.ClaimJob(ctx, dbgen.ClaimJobParams{JobType: jobType, Token: tokenValue(token), LeaseMicroseconds: lease.Microseconds()})
	if errors.Is(err, pgx.ErrNoRows) {
		return multiline.Claim{}, multiline.ErrNoJob
	}
	if err != nil {
		return multiline.Claim{}, fmt.Errorf("claim: %w", err)
	}
	return multiline.Claim{Token: token, Execution: multiline.Execution{
		ID: row.ID, Type: row.Type, Payload: row.Payload, RunAt: row.NextRunTime,
		Timeout: time.Duration(row.TimeoutSeconds) * time.Second, Attempt: row.Attempts,
	}}, nil
}

// Renew extends an unexpired claim. Expired leases cannot be resurrected.
func (s *Store) Renew(ctx context.Context, claim multiline.Claim, lease time.Duration) error {
	if err := validateLease(lease); err != nil {
		return err
	}
	rows, err := s.queries.RenewJob(ctx, dbgen.RenewJobParams{ID: claim.Execution.ID, Token: tokenValue(claim.Token), LeaseMicroseconds: lease.Microseconds()})
	if err != nil {
		return fmt.Errorf("renew: %w", err)
	}
	if rows != 1 {
		return multiline.ErrLeaseLost
	}
	return nil
}

// Finish applies retry policy to the current unexpired attempt and returns the
// persisted outcome. Permanent errors and exhausted attempts fail immediately.
func (s *Store) Finish(ctx context.Context, claim multiline.Claim, executionErr error) (multiline.FinishResult, error) {
	message := multiline.ErrorMessage(executionErr)
	row, err := s.queries.FinishJob(ctx, dbgen.FinishJobParams{ID: claim.Execution.ID, Token: tokenValue(claim.Token), Succeeded: executionErr == nil, Permanent: multiline.IsPermanent(executionErr), LastError: message})
	if errors.Is(err, pgx.ErrNoRows) {
		return multiline.FinishResult{}, multiline.ErrLeaseLost
	}
	if err != nil {
		return multiline.FinishResult{}, fmt.Errorf("finish: %w", err)
	}
	result := multiline.FinishResult{Status: multiline.Status(row.Status)}
	if result.Status == multiline.StatusPending {
		result.RetryAt = &row.NextRunTime
	}
	return result, nil
}

// Recover processes at most limit expired attempts. Retryable work returns to
// pending after jittered backoff; exhausted work becomes failed. It never claims
// jobs and may be called by an independent maintenance process.
func (s *Store) Recover(ctx context.Context, limit int32) (multiline.RecoveryResult, error) {
	if limit < 1 || limit > 1000 {
		return multiline.RecoveryResult{}, invalid("batch_size", "must be between 1 and 1000")
	}
	row, err := s.queries.RecoverJobs(ctx, limit)
	if err != nil {
		return multiline.RecoveryResult{}, fmt.Errorf("recover: %w", err)
	}
	return multiline.RecoveryResult{Retried: row.Retried, Failed: row.Failed}, nil
}

// Prune deletes at most limit terminal jobs completed strictly before before.
// Pending/running jobs are never removed. Applications choose and schedule their
// own retention policy; repeat bounded calls to clear a large history.
func (s *Store) Prune(ctx context.Context, before time.Time, limit int32) (int64, error) {
	if before.IsZero() {
		return 0, invalid("before", "is required")
	}
	if limit < 1 || limit > 1000 {
		return 0, invalid("batch_size", "must be between 1 and 1000")
	}
	count, err := s.queries.PruneJobs(ctx, dbgen.PruneJobsParams{BeforeTime: before, BatchSize: limit})
	if err != nil {
		return 0, fmt.Errorf("prune: %w", err)
	}
	return count, nil
}

// Enqueue persists a job with at most MaxAttempts executions. Durations round up
// to database precision (seconds for Timeout, milliseconds for RetryDelay).
// A duplicate ID returns an error matching multiline.ErrDuplicateJob.
func (s *access) Enqueue(ctx context.Context, job multiline.EnqueueParams) (uuid.UUID, error) {
	normalized, err := job.Normalize()
	if err != nil {
		return uuid.Nil, err
	}
	job = normalized

	err = s.queries.EnqueueJob(ctx, dbgen.EnqueueJobParams{
		ID: job.ID, Type: job.Type, RunAt: job.RunAt, Payload: job.Payload,
		TimeoutSeconds: int32(job.Timeout / time.Second), MaxAttempts: job.MaxAttempts,
		RetryDelayMs:    job.RetryDelay.Milliseconds(),
		RetryMaxDelayMs: job.MaxRetryDelay.Milliseconds(),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" && pgErr.ConstraintName == "jobs_payload_size" {
			return uuid.Nil, fmt.Errorf("enqueue job: %w", errors.Join(invalid("payload", "PostgreSQL JSON representation exceeds 256 KiB"), err))
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.HasSuffix(pgErr.ConstraintName, "_pkey") {
			return uuid.Nil, fmt.Errorf("enqueue job %s: %w", job.ID, errors.Join(multiline.ErrDuplicateJob, err))
		}
		return uuid.Nil, fmt.Errorf("enqueue job: %w", err)
	}
	return job.ID, nil
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

// List returns a creation-ordered cursor page. Retry scheduling cannot move
// records across its boundary. It is not a transactional snapshot across calls.
func (s *access) List(ctx context.Context, params multiline.ListParams) (multiline.JobPage, error) {
	if params.Limit == 0 {
		params.Limit = 100
	}
	if params.Limit < 1 || params.Limit > 1000 {
		return multiline.JobPage{}, invalid("limit", "must be between 1 and 1000")
	}
	var rows []dbgen.Job
	var err error
	if params.After == nil {
		rows, err = s.queries.ListJobs(ctx, params.Limit+1)
	} else {
		if params.After.CreatedAt.IsZero() || params.After.ID == uuid.Nil {
			return multiline.JobPage{}, invalid("after", "must be a complete cursor returned by List")
		}
		rows, err = s.queries.ListJobsAfter(ctx, dbgen.ListJobsAfterParams{PageLimit: params.Limit + 1, AfterCreatedAt: params.After.CreatedAt, AfterID: params.After.ID})
	}
	if err != nil {
		return multiline.JobPage{}, fmt.Errorf("list jobs: %w", err)
	}
	page := multiline.JobPage{Jobs: make([]multiline.JobRecord, 0, min(int(params.Limit), len(rows)))}
	if len(rows) > int(params.Limit) {
		rows = rows[:params.Limit]
		last := rows[len(rows)-1]
		page.Next = &multiline.JobCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	for _, row := range rows {
		page.Jobs = append(page.Jobs, record(row))
	}
	return page, nil
}

func record(row dbgen.Job) multiline.JobRecord {
	return multiline.JobRecord{
		ID: row.ID, Type: row.Type, Payload: row.Payload, RunAt: row.NextRunTime,
		Timeout: time.Duration(row.TimeoutSeconds) * time.Second, Status: multiline.Status(row.Status),
		Attempts: row.Attempts, MaxAttempts: row.MaxAttempts, RetryDelay: time.Duration(row.RetryDelayMs) * time.Millisecond,
		MaxRetryDelay: time.Duration(row.RetryMaxDelayMs) * time.Millisecond,
		LastError:     row.LastError, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		StartedAt: optionalTime(row.StartedAt), CompletedAt: optionalTime(row.CompletedAt), LeaseExpiresAt: optionalTime(row.LeaseExpiresAt),
	}
}
