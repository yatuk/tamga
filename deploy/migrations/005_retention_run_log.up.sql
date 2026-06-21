-- FAZ2 — retention job observability (request_logs is already partitioned in 001).

CREATE TABLE IF NOT EXISTS retention_run_log (
    id BIGSERIAL PRIMARY KEY,
    ran_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    phase TEXT NOT NULL,
    message TEXT,
    detail JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_retention_run_log_ran_at ON retention_run_log (ran_at DESC);
