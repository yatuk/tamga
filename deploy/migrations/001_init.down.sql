-- Tamga initial schema — rollback
-- Migration: 001_init.down.sql

DROP TABLE IF EXISTS alerts CASCADE;
DROP TABLE IF EXISTS daily_stats CASCADE;
DROP TABLE IF EXISTS request_logs CASCADE;
DROP TABLE IF EXISTS policies CASCADE;
DROP TABLE IF EXISTS api_keys CASCADE;
DROP TABLE IF EXISTS organizations CASCADE;
