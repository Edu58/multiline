-- name: ListJobs :many
SELECT * FROM jobs 
ORDER BY id 
LIMIT $1 OFFSET $2;

-- name: GetJob :one
SELECT * FROM jobs WHERE id = $1;

-- name: CreateOrUpdateJob :one
INSERT INTO jobs (id, type, name, description, schedule, last_run_time, next_run_time, payload, status, shard_id) 
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: UpdateJobNextRunTimeAndStatus :exec
UPDATE jobs 
SET next_run_time = $2, 
    payload = $3, 
    status = $4 
WHERE id = $1;

-- name: DeleteJob :exec
DELETE FROM jobs WHERE id = $1;
