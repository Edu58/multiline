CREATE TABLE jobs (
    id uuid PRIMARY KEY,
    type text NOT NULL CHECK (octet_length(type) BETWEEN 1 AND 20 AND type ~ '[^[:space:]]'),
    payload jsonb CONSTRAINT jobs_payload_size CHECK (octet_length(payload::text) <= 262144),
    next_run_time timestamptz NOT NULL,
    timeout_seconds integer NOT NULL DEFAULT 15 CHECK (timeout_seconds > 0),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','completed','failed')),
    attempts integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 1 CHECK (max_attempts BETWEEN 1 AND 1000),
    retry_delay_ms bigint NOT NULL DEFAULT 1000 CHECK (retry_delay_ms BETWEEN 1 AND 86400000),
    retry_max_delay_ms bigint NOT NULL DEFAULT 3600000 CHECK (retry_max_delay_ms BETWEEN retry_delay_ms AND 86400000),
    claim_token uuid,
    lease_expires_at timestamptz,
    last_error text NOT NULL DEFAULT '' CONSTRAINT jobs_error_size CHECK (octet_length(last_error) <= 4096),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    started_at timestamptz,
    completed_at timestamptz,
    CONSTRAINT jobs_attempts_valid CHECK (attempts BETWEEN 0 AND max_attempts),
    CONSTRAINT jobs_state_attempts_valid CHECK (
        (status = 'pending' AND attempts < max_attempts)
        OR (status <> 'pending' AND attempts > 0 AND started_at IS NOT NULL)
    ),
    CONSTRAINT jobs_lease_valid CHECK (
        (status = 'running' AND claim_token IS NOT NULL AND lease_expires_at IS NOT NULL AND started_at IS NOT NULL AND attempts > 0)
        OR (status <> 'running' AND claim_token IS NULL AND lease_expires_at IS NULL)
    ),
    CONSTRAINT jobs_completion_valid CHECK ((status IN ('completed','failed')) = (completed_at IS NOT NULL))
);
CREATE INDEX jobs_pending_due ON jobs (type, next_run_time, id) WHERE status = 'pending';
CREATE INDEX jobs_expired_leases ON jobs (lease_expires_at, id) WHERE status = 'running';
CREATE INDEX jobs_creation_order ON jobs (created_at, id);
CREATE INDEX jobs_terminal_retention ON jobs (completed_at, id) WHERE status IN ('completed','failed');
