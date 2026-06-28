-- Output scan — rollback
-- Migration: 003_output_scan.down.sql

DROP INDEX IF EXISTS idx_request_logs_output_action;
