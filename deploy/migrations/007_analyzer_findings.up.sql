-- Analyzer findings persistence table
-- Migration: 007_analyzer_findings.up.sql
--
-- Previously this DDL was executed inline on every write path
-- (store_results in analyzer/app/main.py). Each call issued
-- CREATE TABLE IF NOT EXISTS which acquires AccessExclusiveLock,
-- serialising all concurrent writes. Moved here permanently.

CREATE TABLE IF NOT EXISTS analyzer_findings (
    id            BIGSERIAL PRIMARY KEY,
    request_id    TEXT NOT NULL,
    org_id        TEXT,
    finding_type  TEXT NOT NULL,
    category      TEXT NOT NULL,
    severity      TEXT NOT NULL,
    match_text    TEXT,
    confidence    REAL NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_af_request_id ON analyzer_findings(request_id);
CREATE INDEX IF NOT EXISTS idx_af_org_id    ON analyzer_findings(org_id);
CREATE INDEX IF NOT EXISTS idx_af_created_at ON analyzer_findings(created_at);
