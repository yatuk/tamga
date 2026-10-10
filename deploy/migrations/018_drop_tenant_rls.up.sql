-- 018_drop_tenant_rls: remove the row-level security added in 011.
--
-- The policy compared org_id with the setting app.tenant_id, which nothing
-- in the proxy ever set. It had no effect in any deployment: the proxy
-- connects as the role that owns the tables, and an owner is not subject to
-- a policy unless the table is FORCEd. With a role that was subject to it,
-- every read and write of these tables would have failed, because the
-- setting is unset.
--
-- What keeps one organisation's rows from another is the org_id predicate
-- in each query. A policy that reads as protection and does nothing is
-- worse than none, so it goes. Bringing it back for real needs the proxy to
-- set the tenant per transaction and a separate role for maintenance.

DROP POLICY IF EXISTS tenant_isolation ON request_logs;
DROP POLICY IF EXISTS tenant_isolation ON daily_stats;

ALTER TABLE IF EXISTS request_logs DISABLE ROW LEVEL SECURITY;
ALTER TABLE IF EXISTS daily_stats DISABLE ROW LEVEL SECURITY;

DROP FUNCTION IF EXISTS set_tenant(text);
