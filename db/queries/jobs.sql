-- name: CreateOrUpdateJob :one
INSERT INTO jobs (type, name, description, schedule, last_run_time, next_run_time, payload, shard_id, started_at, completed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: UpdateJobStartedAt :exec
UPDATE jobs
SET started_at = $2,
status = $3
WHERE id = $1;

-- name: UpdateJobCompletedAt :exec
UPDATE jobs
SET completed_at = $2,
next_run_time = $3,
status = $4,
last_run_result = $5
WHERE id = $1;

-- name: UpdateJobShardIs :exec
UPDATE jobs
SET shard_id = $2
WHERE id = $1;

-- name: DeleteJob :exec
DELETE FROM jobs WHERE id = $1;

-- name: ListJobs :many
SELECT * FROM jobs
ORDER BY next_run_time ASC
LIMIT $1 OFFSET $2;

-- name: GetJob :one
SELECT * FROM jobs
WHERE id = $1
ORDER BY next_run_time ASC;

-- name: GetNextMinuteJobs :many
SELECT *
FROM jobs
WHERE status = sqlc.arg(status)
  AND next_run_time < sqlc.arg(end_time)
ORDER BY next_run_time ASC;

-- name: GetJobsByWindow :many
SELECT *
FROM jobs
WHERE status = sqlc.arg(status)
  AND next_run_time >= sqlc.arg(start_time)
  AND next_run_time < sqlc.arg(end_time)
ORDER BY next_run_time ASC;
