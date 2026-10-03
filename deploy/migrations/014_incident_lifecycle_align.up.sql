-- 014_incident_lifecycle_align: bring incident_lifecycle in line with the
-- proxy's incident store. Migration 012 created the table without org_id and
-- comments and with nullable text columns, while the store selects those
-- columns and scans every text column as a non-null string. On a database
-- initialised from 012, triage, resolution and the MTTR report all failed
-- with "column org_id does not exist".
--
-- Idempotent: safe on tables created by 012 and on tables the proxy created
-- itself.

ALTER TABLE incident_lifecycle ADD COLUMN IF NOT EXISTS org_id TEXT NOT NULL DEFAULT '';
ALTER TABLE incident_lifecycle ADD COLUMN IF NOT EXISTS comments JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE incident_lifecycle
SET assignee         = COALESCE(assignee, ''),
    reason           = COALESCE(reason, ''),
    tags             = COALESCE(tags, '{}'),
    triaged_by       = COALESCE(triaged_by, ''),
    resolved_by      = COALESCE(resolved_by, ''),
    resolution       = COALESCE(resolution, ''),
    resolution_notes = COALESCE(resolution_notes, '')
WHERE assignee IS NULL OR reason IS NULL OR tags IS NULL OR triaged_by IS NULL
   OR resolved_by IS NULL OR resolution IS NULL OR resolution_notes IS NULL;

ALTER TABLE incident_lifecycle
    ALTER COLUMN assignee SET DEFAULT '',         ALTER COLUMN assignee SET NOT NULL,
    ALTER COLUMN reason SET DEFAULT '',           ALTER COLUMN reason SET NOT NULL,
    ALTER COLUMN tags SET DEFAULT '{}',           ALTER COLUMN tags SET NOT NULL,
    ALTER COLUMN triaged_by SET DEFAULT '',       ALTER COLUMN triaged_by SET NOT NULL,
    ALTER COLUMN resolved_by SET DEFAULT '',      ALTER COLUMN resolved_by SET NOT NULL,
    ALTER COLUMN resolution SET DEFAULT '',       ALTER COLUMN resolution SET NOT NULL,
    ALTER COLUMN resolution_notes SET DEFAULT '', ALTER COLUMN resolution_notes SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_il_org ON incident_lifecycle(org_id, updated_at DESC);
