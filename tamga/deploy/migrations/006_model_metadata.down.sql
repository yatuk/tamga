-- Model metadata — rollback
-- Migration: 006_model_metadata.down.sql

DROP INDEX IF EXISTS idx_request_logs_model_family;
