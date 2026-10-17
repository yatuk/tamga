-- Migration: 021_classifier_scores.up.sql
-- The inline classifier's scores and its shadow model's, text by text.
--
-- Safe on a table that has rows, and safe to run again.

-- Written only for a request scored while the classifier service ran a
-- shadow model (TAMGA_CLASSIFIER_SHADOW_MODEL_DIR): the model in service and
-- the candidate, each text's score from both, and the thresholds they were
-- read at. No text is stored. NULL on every other row, and on every row
-- written before.
--
-- Adding a nullable column with no default changes the catalog only: the
-- table is not rewritten. A proxy of this version adds the column itself at
-- startup, so this is then a no-op.
ALTER TABLE request_logs
    ADD COLUMN IF NOT EXISTS classifier_scores JSONB;
