-- Migration: 020_route_pool.up.sql
-- The internal pool a ROUTE verdict named for a request.
--
-- Safe on a table that has rows, and safe to run again.

-- With action_taken 'route' the request was answered by that pool. With
-- 'block' the pool could not take it and the request was stopped. NULL on
-- every other row, and on every row written before.
--
-- Adding a nullable column with no default changes the catalog only: the
-- table is not rewritten. A proxy of this version adds the column itself at
-- startup, so this is then a no-op.
ALTER TABLE request_logs
    ADD COLUMN IF NOT EXISTS route_pool TEXT;
