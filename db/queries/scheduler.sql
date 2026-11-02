-- name: ClaimJob :one
UPDATE jobs
SET status = 'running', started_at = clock_timestamp(), updated_at = clock_timestamp(),
    attempts = attempts + 1, claim_token = sqlc.arg(token),
    lease_expires_at = clock_timestamp() + sqlc.arg(lease_microseconds)::bigint * interval '1 microsecond'
WHERE id = (
    SELECT id FROM jobs
    WHERE status = 'pending' AND attempts < max_attempts AND next_run_time <= clock_timestamp()
    ORDER BY next_run_time, id FOR UPDATE SKIP LOCKED LIMIT 1
)
RETURNING id, type, payload, next_run_time, timeout_seconds, attempts;

-- name: RenewJob :execrows
UPDATE jobs SET lease_expires_at = clock_timestamp() + sqlc.arg(lease_microseconds)::bigint * interval '1 microsecond',
    updated_at = clock_timestamp()
WHERE id = sqlc.arg(id) AND status = 'running'
    AND claim_token = sqlc.arg(token) AND lease_expires_at > clock_timestamp();

-- name: FinishJob :one
UPDATE jobs
SET status = CASE WHEN sqlc.arg(succeeded)::boolean THEN 'completed'
                  WHEN sqlc.arg(permanent)::boolean OR attempts >= max_attempts THEN 'failed' ELSE 'pending' END,
    completed_at = CASE WHEN sqlc.arg(succeeded)::boolean OR sqlc.arg(permanent)::boolean OR attempts >= max_attempts
                        THEN clock_timestamp() ELSE NULL END,
    next_run_time = CASE WHEN NOT sqlc.arg(succeeded)::boolean AND NOT sqlc.arg(permanent)::boolean AND attempts < max_attempts
        THEN clock_timestamp() + (1 + floor(random() * least(retry_max_delay_ms::double precision,
             retry_delay_ms::double precision * power(2::double precision, least(attempts - 1, 30))))) * interval '1 millisecond'
        ELSE next_run_time END,
    updated_at = clock_timestamp(), claim_token = NULL, lease_expires_at = NULL,
    last_error = sqlc.arg(last_error)::text
WHERE id = sqlc.arg(id) AND status = 'running'
    AND claim_token = sqlc.arg(token) AND lease_expires_at > clock_timestamp()
RETURNING status, next_run_time;

-- name: RecoverJobs :one
WITH recovered AS (
    UPDATE jobs
    SET status = CASE WHEN attempts < max_attempts THEN 'pending' ELSE 'failed' END,
        completed_at = CASE WHEN attempts < max_attempts THEN NULL ELSE clock_timestamp() END,
        next_run_time = CASE WHEN attempts < max_attempts
            THEN clock_timestamp() + (1 + floor(random() * least(retry_max_delay_ms::double precision,
                 retry_delay_ms::double precision * power(2::double precision, least(attempts - 1, 30))))) * interval '1 millisecond'
            ELSE next_run_time END,
        updated_at = clock_timestamp(), claim_token = NULL, lease_expires_at = NULL,
        last_error = 'lease expired'
    WHERE id IN (
        SELECT id FROM jobs WHERE status = 'running' AND lease_expires_at <= clock_timestamp()
        ORDER BY lease_expires_at, id FOR UPDATE SKIP LOCKED LIMIT sqlc.arg(batch_size)
    )
    RETURNING status
)
SELECT count(*) FILTER (WHERE status = 'pending')::bigint AS retried,
       count(*) FILTER (WHERE status = 'failed')::bigint AS failed FROM recovered;

-- name: EnqueueJob :exec
INSERT INTO jobs (id, type, next_run_time, payload, timeout_seconds, max_attempts, retry_delay_ms, retry_max_delay_ms)
VALUES (sqlc.arg(id), sqlc.arg(type), sqlc.arg(run_at), sqlc.narg(payload),
        sqlc.arg(timeout_seconds), sqlc.arg(max_attempts), sqlc.arg(retry_delay_ms), sqlc.arg(retry_max_delay_ms));

-- name: ListJobs :many
SELECT * FROM jobs ORDER BY created_at, id LIMIT sqlc.arg(page_limit);

-- name: ListJobsAfter :many
SELECT * FROM jobs WHERE (created_at, id) > (sqlc.arg(after_created_at)::timestamptz, sqlc.arg(after_id)::uuid)
ORDER BY created_at, id LIMIT sqlc.arg(page_limit);

-- name: PruneJobs :execrows
DELETE FROM jobs WHERE id IN (
    SELECT id FROM jobs WHERE status IN ('completed','failed') AND completed_at < sqlc.arg(before_time)::timestamptz
    ORDER BY completed_at, id FOR UPDATE SKIP LOCKED LIMIT sqlc.arg(batch_size)
);
