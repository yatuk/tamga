-- Request record — rollback
-- Migration: 017_request_record.down.sql
--
-- Dropping the columns discards what was stored in them. A running proxy
-- notices and goes on without them, and adds them again at its next start.

DROP INDEX IF EXISTS idx_request_logs_org_request;

ALTER TABLE IF EXISTS request_logs
    DROP COLUMN IF EXISTS output_risk,
    DROP COLUMN IF EXISTS policy_version,
    DROP COLUMN IF EXISTS policy_name;
