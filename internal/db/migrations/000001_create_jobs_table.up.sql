CREATE TABLE jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(40) NOT NULL,
    description TEXT,
    type VARCHAR(20) NOT NULL,
    schedule VARCHAR(50) NOT NULL,
    last_run_time TIMESTAMPTZ,
    next_run_time TIMESTAMPTZ,
    payload JSON,
    status VARCHAR(20) DEFAULT 'pending',
    shard_id INT,
    inserted_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_jobs_shard_id_next_run_time ON jobs(shard_id, next_run_time);
CREATE INDEX idx_jobs_type ON jobs(type);
CREATE INDEX idx_jobs_name ON jobs(name);
CREATE INDEX idx_jobs_status ON jobs(status);
CREATE INDEX idx_jobs_shard_id ON jobs(shard_id);
