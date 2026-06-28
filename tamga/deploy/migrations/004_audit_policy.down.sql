-- Audit + policy tables — rollback
-- Migration: 004_audit_policy.down.sql

DROP TABLE IF EXISTS policy_proposals CASCADE;
DROP TABLE IF EXISTS policy_revisions CASCADE;
DROP TABLE IF EXISTS audit_log CASCADE;
