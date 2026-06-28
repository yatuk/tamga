-- Additional performance indexes — rollback
-- Migration: 002_indexes.down.sql

DROP INDEX CONCURRENTLY IF EXISTS idx_request_logs_created_at_brin;
DROP INDEX CONCURRENTLY IF EXISTS idx_api_keys_org_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_alerts_org_severity;
DROP INDEX CONCURRENTLY IF EXISTS idx_daily_stats_stat_date_brin;
