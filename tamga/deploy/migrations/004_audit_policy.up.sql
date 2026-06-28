-- 004_audit_policy.sql — persistent audit chain + policy history.
-- Adds durable storage for tamper-evident audit entries and for
-- policy revisions / proposals so that a dashboard running against a
-- fresh replica still sees the full compliance trail.

CREATE TABLE IF NOT EXISTS audit_log (
    id          BIGSERIAL PRIMARY KEY,
    ts          TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor       TEXT,
    kind        TEXT NOT NULL,
    target      TEXT,
    detail      JSONB NOT NULL DEFAULT '{}'::jsonb,
    prev_hash   TEXT,
    hash        TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_log_ts   ON audit_log (ts DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_kind ON audit_log (kind);

CREATE TABLE IF NOT EXISTS policy_revisions (
    id          UUID PRIMARY KEY,
    ts          TIMESTAMPTZ NOT NULL DEFAULT now(),
    author      TEXT NOT NULL,
    message     TEXT,
    yaml        TEXT NOT NULL,
    parent_id   UUID
);

CREATE INDEX IF NOT EXISTS idx_policy_revisions_ts ON policy_revisions (ts DESC);

CREATE TABLE IF NOT EXISTS policy_proposals (
    id            UUID PRIMARY KEY,
    ts            TIMESTAMPTZ NOT NULL DEFAULT now(),
    author        TEXT NOT NULL,
    message       TEXT,
    yaml          TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'open',
    approved_by   TEXT,
    approved_at   TIMESTAMPTZ,
    rejected_by   TEXT,
    rejected_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_policy_proposals_status ON policy_proposals (status);
CREATE INDEX IF NOT EXISTS idx_policy_proposals_ts     ON policy_proposals (ts DESC);
