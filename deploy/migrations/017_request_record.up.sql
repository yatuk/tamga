-- Migration: 017_request_record.up.sql
-- What a stored request could not say about itself, and a way to find one by
-- its id.
--
-- Safe on a table that has rows, and safe to run again.

-- The policy in force when the request was decided, and the risk of the
-- response as the proxy scored it. NULL on every row written before: the
-- console leaves these fields out for such a request.
--
-- Adding a nullable column with no default changes the catalog only. The
-- table is not rewritten; the ACCESS EXCLUSIVE lock on request_logs and each
-- of its partitions is held for the moment the change takes (it does wait
-- for transactions that are using the table). A proxy of this version adds
-- the same three columns itself at startup, so this is then a no-op.
ALTER TABLE request_logs
    ADD COLUMN IF NOT EXISTS policy_name TEXT,
    ADD COLUMN IF NOT EXISTS policy_version TEXT,
    ADD COLUMN IF NOT EXISTS output_risk JSONB;

-- A request is looked up by its id within one organisation, newest first
-- (GET /api/v1/events/{request_id}), and the writer asks whether an event's
-- row is already there before it adds one. Without this index both walked
-- every row of the organisation in the window.
--
-- Not CONCURRENTLY: request_logs is partitioned (see 002). A plain CREATE
-- INDEX on the parent builds the index on every partition, the default one
-- included, and partitions created later get it by themselves. It takes a
-- SHARE lock on the table and its partitions for as long as the build reads
-- them: reads go on, inserts wait. The proxy buffers rows in memory and on
-- NATS meanwhile, so on a large table run this in a quiet hour, or build it
-- without blocking writes, by hand, before this file:
--
--   CREATE INDEX idx_request_logs_org_request ON ONLY request_logs
--       (org_id, request_id, created_at DESC);
--   -- then, for each partition P (\d+ request_logs lists them):
--   CREATE INDEX CONCURRENTLY P_org_request ON P (org_id, request_id, created_at DESC);
--   ALTER INDEX idx_request_logs_org_request ATTACH PARTITION P_org_request;
--
-- after which the statement below finds the index and does nothing.
CREATE INDEX IF NOT EXISTS idx_request_logs_org_request
    ON request_logs (org_id, request_id, created_at DESC);

-- scan_latency_ms and total_latency_ms need no change: they are REAL, and
-- hold fractions of a millisecond. It was the proxy that sent whole ones.
