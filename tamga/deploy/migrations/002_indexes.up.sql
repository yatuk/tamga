-- Additional performance indexes
-- Migration: 002_indexes.up.sql

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_request_logs_created_at_brin
    ON request_logs USING BRIN (created_at);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_api_keys_org_id
    ON api_keys(org_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_alerts_org_severity
    ON alerts(org_id, severity, created_at DESC)
    WHERE is_read = false;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_daily_stats_stat_date_brin
    ON daily_stats USING BRIN (stat_date);
