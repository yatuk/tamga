-- 019_drop_unused_tables: drop five tables that nothing reads or writes.
--
--   api_keys      (001)  Keys live in virtual_keys (015). Never queried.
--   policies      (001)  The policy is a file; its history is in
--                        policy_revisions and policy_proposals (004).
--   alerts        (001)  Never written. The retention job deleted old rows
--                        from it and nothing else touched it.
--   daily_stats   (001)  Never written. Counts and trends are read from
--                        request_logs.
--   event_outbox  (010)  Created for an outbox worker that was not built.
--
-- No version of the proxy or the analyzer inserts into any of them, so they
-- are expected to be empty. THIS DROPS WHATEVER THEY HOLD. If you put rows
-- into one yourself and want them, dump it first:
--
--   pg_dump "$TAMGA_DB_URL" --data-only -t api_keys -t policies -t alerts \
--     -t daily_stats -t event_outbox > unused_tables.sql
--
-- Upgrade the proxy before running this. An older proxy's retention job
-- deletes from alerts and daily_stats and fails its purge step once they are
-- gone.
--
-- Each DROP takes its table's indexes and its foreign key to organizations
-- with it; nothing else depends on these tables, so there is no CASCADE and
-- a dependent object you added yourself stops the statement instead of being
-- dropped. The locks are on the dropped tables and, for an instant, on
-- organizations. Safe to run again.

DROP TABLE IF EXISTS event_outbox;
DROP TABLE IF EXISTS alerts;
DROP TABLE IF EXISTS daily_stats;
DROP TABLE IF EXISTS policies;
DROP TABLE IF EXISTS api_keys;
