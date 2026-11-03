
ALTER TABLE refunds DROP COLUMN IF EXISTS idempotency_key;
ALTER TABLE refunds DROP COLUMN IF EXISTS attempts;

CREATE TABLE IF NOT EXISTS refund_jobs (
    id              uuid PRIMARY KEY,
    refund_id       uuid NOT NULL,
    job_type        varchar(20) NOT NULL,
    next_run_at     timestamptz NOT NULL,
    create_attempts integer NOT NULL DEFAULT 0,
    query_attempts  integer NOT NULL DEFAULT 0,
    last_error      varchar(500) NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uni_refund_jobs_refund_id ON refund_jobs (refund_id);
CREATE INDEX IF NOT EXISTS idx_refund_jobs_next_run ON refund_jobs (next_run_at);
