-- Route pool — rollback
-- Migration: 020_route_pool.down.sql
--
-- Dropping the column discards what was stored in it. A running proxy
-- notices and goes on without it, and adds it again at its next start.

ALTER TABLE IF EXISTS request_logs
    DROP COLUMN IF EXISTS route_pool;
