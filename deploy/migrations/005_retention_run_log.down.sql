-- Retention run log — rollback
-- Migration: 005_retention_run_log.down.sql

DROP TABLE IF EXISTS retention_run_log CASCADE;
