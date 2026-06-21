-- 013_saved_hunts: server-side saved threat-hunting queries.
-- org-scoped so hunts survive across devices and browsers.
-- Replaces dashboard localStorage with PostgreSQL persistence.

CREATE TABLE IF NOT EXISTS saved_hunts (
    id         UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    org_id     TEXT NOT NULL,
    name       TEXT NOT NULL,
    query_json JSONB NOT NULL,
    created_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_saved_hunts_org ON saved_hunts(org_id, created_at DESC);
