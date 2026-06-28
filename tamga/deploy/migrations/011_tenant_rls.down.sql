DROP POLICY IF EXISTS tenant_isolation ON request_logs;
DROP POLICY IF EXISTS tenant_isolation ON daily_stats;
ALTER TABLE request_logs DISABLE ROW LEVEL SECURITY;
ALTER TABLE daily_stats DISABLE ROW LEVEL SECURITY;
DROP FUNCTION IF EXISTS set_tenant(text);
