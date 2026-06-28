-- Tamga initial schema
-- Migration: 001_init.sql

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Organizations
CREATE TABLE organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    plan TEXT NOT NULL DEFAULT 'trial',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- API Keys (proxy authentication)
CREATE TABLE api_keys (
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

CREATE INDEX idx_api_keys_prefix ON api_keys(key_prefix) WHERE is_active = true;

-- Policies
CREATE TABLE policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    config JSONB NOT NULL DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_policies_org_active ON policies(org_id) WHERE is_active = true;

-- Request Logs (partitioned by month for performance)
CREATE TABLE request_logs (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL,
    request_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    model TEXT,
    endpoint TEXT,
    input_tokens INT DEFAULT 0,
    output_tokens INT DEFAULT 0,
    findings JSONB NOT NULL DEFAULT '[]',
    findings_count INT NOT NULL DEFAULT 0,
    action_taken TEXT NOT NULL DEFAULT 'pass',
    scan_latency_ms REAL DEFAULT 0,
    total_latency_ms REAL DEFAULT 0,
    user_identifier TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Create partitions for current and next month
CREATE TABLE request_logs_default PARTITION OF request_logs DEFAULT;

CREATE INDEX idx_request_logs_org_time ON request_logs(org_id, created_at DESC);
CREATE INDEX idx_request_logs_action ON request_logs(action_taken, created_at DESC);
CREATE INDEX idx_request_logs_findings ON request_logs USING GIN(findings) WHERE findings_count > 0;

-- Analytics: daily aggregates (materialized for dashboard speed)
CREATE TABLE daily_stats (
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

CREATE INDEX idx_daily_stats_org_date ON daily_stats(org_id, stat_date DESC);

-- Alerts
CREATE TABLE alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    request_id TEXT NOT NULL,
    severity TEXT NOT NULL,
    finding_type TEXT NOT NULL,
    message TEXT NOT NULL,
    is_read BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_alerts_org_unread ON alerts(org_id, created_at DESC) WHERE is_read = false;

-- Seed: default organization for initial setup.
-- API keys must be created through the admin API after first boot —
-- no hardcoded keys in migrations.
INSERT INTO organizations (name, slug, plan) VALUES ('Default Org', 'default', 'trial')
ON CONFLICT DO NOTHING;
