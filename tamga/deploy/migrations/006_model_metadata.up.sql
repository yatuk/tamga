-- Migration: 006_model_metadata.sql
-- Adds model_family for grouping model variants in dashboards.

ALTER TABLE request_logs ADD COLUMN IF NOT EXISTS model_family TEXT;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_request_logs_model_family
    ON request_logs(org_id, model_family, created_at DESC)
    WHERE model_family IS NOT NULL;
