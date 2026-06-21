-- 012_incident_lifecycle: persists incident triage and resolution state.
-- Tracks lifecycle from Open through triage to resolution, with assignee,
-- tags, reason, and resolution metadata.

CREATE TABLE IF NOT EXISTS incident_lifecycle (
    request_id   TEXT PRIMARY KEY,
    status       TEXT NOT NULL DEFAULT 'Open',
    assignee     TEXT,
    reason       TEXT,
    tags         TEXT[],
    triaged_at   TIMESTAMPTZ,
    triaged_by   TEXT,
    resolved_at  TIMESTAMPTZ,
    resolved_by  TEXT,
    resolution   TEXT,
    resolution_notes TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_il_resolved ON incident_lifecycle(resolution, resolved_at) WHERE resolved_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_il_status ON incident_lifecycle(status);
