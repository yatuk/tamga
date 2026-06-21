-- 011_tenant_rls: row-level security for tenant isolation.
-- Enforces org-scoped access at the database level so that a compromised
-- query or bug cannot leak cross-tenant data.

ALTER TABLE request_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE daily_stats ENABLE ROW LEVEL SECURITY;

-- Tenant isolation policy: restricts rows to the current tenant.
-- The application sets app.tenant_id at session/connection start via:
--   SELECT set_config('app.tenant_id', $1, false)
CREATE POLICY tenant_isolation ON request_logs
    FOR ALL
    USING (org_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY tenant_isolation ON daily_stats
    FOR ALL
    USING (org_id = current_setting('app.tenant_id')::uuid);

-- Helper to switch tenant context within a session.
CREATE OR REPLACE FUNCTION set_tenant(tenant_id text) RETURNS void AS $$
BEGIN
    PERFORM set_config('app.tenant_id', tenant_id, false);
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
