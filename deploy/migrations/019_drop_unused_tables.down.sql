-- Reverses 019: recreates the five tables, empty, as migrations 001, 002 and
-- 010 defined them. The rows they held are not brought back.
--
-- The indexes that 002 built CONCURRENTLY are built plainly here: the tables
-- are new and empty. daily_stats comes back without row-level security,
-- which is how 018 left it; rolling 018 back afterwards adds it again.

-- API Keys (proxy authentication)
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    key_hash TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT 'default',
    scopes TEXT[] DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_api_keys_org_id
    ON api_keys(org_id);

-- Policies
CREATE TABLE IF NOT EXISTS policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_policies_org_active ON policies(org_id) WHERE is_active = true;

-- Analytics: daily aggregates (materialized for dashboard speed)
CREATE TABLE IF NOT EXISTS daily_stats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    stat_date DATE NOT NULL,
    total_requests INT NOT NULL DEFAULT 0,
    blocked_requests INT NOT NULL DEFAULT 0,
    redacted_requests INT NOT NULL DEFAULT 0,
    warned_requests INT NOT NULL DEFAULT 0,
    total_input_tokens BIGINT NOT NULL DEFAULT 0,
    total_output_tokens BIGINT NOT NULL DEFAULT 0,
    unique_users INT NOT NULL DEFAULT 0,
    provider_breakdown JSONB NOT NULL DEFAULT '{}',
    finding_breakdown JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, stat_date)
);

CREATE INDEX IF NOT EXISTS idx_daily_stats_org_date ON daily_stats(org_id, stat_date DESC);
CREATE INDEX IF NOT EXISTS idx_daily_stats_stat_date_brin
    ON daily_stats USING BRIN (stat_date);

-- Alerts
CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    request_id TEXT NOT NULL,
    severity TEXT NOT NULL,
    finding_type TEXT NOT NULL,
    message TEXT NOT NULL,
    is_read BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alerts_org_unread ON alerts(org_id, created_at DESC) WHERE is_read = false;
CREATE INDEX IF NOT EXISTS idx_alerts_org_severity
    ON alerts(org_id, severity, created_at DESC)
    WHERE is_read = false;

-- Event outbox (010)
CREATE TABLE IF NOT EXISTS event_outbox (
    id          BIGSERIAL PRIMARY KEY,
    event_type  TEXT        NOT NULL,
    payload     JSONB       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    error_count INT         NOT NULL DEFAULT 0,
    last_error  TEXT
);

CREATE INDEX IF NOT EXISTS idx_outbox_pending ON event_outbox (created_at)
    WHERE published_at IS NULL AND error_count < 10;
