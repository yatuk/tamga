-- Reverts 014: back to the column set and nullability of migration 012.
DROP INDEX IF EXISTS idx_il_org;

ALTER TABLE incident_lifecycle
    ALTER COLUMN assignee DROP NOT NULL,         ALTER COLUMN assignee DROP DEFAULT,
    ALTER COLUMN reason DROP NOT NULL,           ALTER COLUMN reason DROP DEFAULT,
    ALTER COLUMN tags DROP NOT NULL,             ALTER COLUMN tags DROP DEFAULT,
    ALTER COLUMN triaged_by DROP NOT NULL,       ALTER COLUMN triaged_by DROP DEFAULT,
    ALTER COLUMN resolved_by DROP NOT NULL,      ALTER COLUMN resolved_by DROP DEFAULT,
    ALTER COLUMN resolution DROP NOT NULL,       ALTER COLUMN resolution DROP DEFAULT,
    ALTER COLUMN resolution_notes DROP NOT NULL, ALTER COLUMN resolution_notes DROP DEFAULT;

ALTER TABLE incident_lifecycle DROP COLUMN IF EXISTS comments;
ALTER TABLE incident_lifecycle DROP COLUMN IF EXISTS org_id;
