-- Puts back what 011 created. See 018_drop_tenant_rls.up.sql for why it
-- does not protect anything as the proxy is written.

ALTER TABLE request_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE daily_stats ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON request_logs;
CREATE POLICY tenant_isolation ON request_logs
    FOR ALL
    USING (org_id = current_setting('app.tenant_id')::uuid);

DROP POLICY IF EXISTS tenant_isolation ON daily_stats;
CREATE POLICY tenant_isolation ON daily_stats
    FOR ALL
    USING (org_id = current_setting('app.tenant_id')::uuid);

CREATE OR REPLACE FUNCTION set_tenant(tenant_id text) RETURNS void AS $$
BEGIN
    PERFORM set_config('app.tenant_id', tenant_id, false);
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
