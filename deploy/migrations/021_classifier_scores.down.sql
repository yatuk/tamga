-- Classifier scores — rollback
-- Migration: 021_classifier_scores.down.sql
--
-- Dropping the column discards what was stored in it. A running proxy
-- notices and goes on without it, and adds it again at its next start.

ALTER TABLE IF EXISTS request_logs
    DROP COLUMN IF EXISTS classifier_scores;
