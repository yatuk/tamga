-- 003_output_scan.sql — adds response-scan artefacts to request_logs.
-- Run after 001_init.sql. Safe to apply on a running cluster: all additions
-- are NULLable and default to NULL for existing rows.

ALTER TABLE IF EXISTS request_logs
    ADD COLUMN IF NOT EXISTS output_findings JSONB,
    ADD COLUMN IF NOT EXISTS output_action TEXT,
    ADD COLUMN IF NOT EXISTS output_tokens INT,
    ADD COLUMN IF NOT EXISTS input_tokens INT,
    ADD COLUMN IF NOT EXISTS cost_usd NUMERIC(12, 6),
    ADD COLUMN IF NOT EXISTS policy_channel TEXT,
    ADD COLUMN IF NOT EXISTS cache_status TEXT;

CREATE INDEX IF NOT EXISTS idx_request_logs_output_action
    ON request_logs (output_action)
    WHERE output_action IS NOT NULL;
