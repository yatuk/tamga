-- Model pricing — rollback
-- Migration: 008_model_pricing.down.sql

DROP TRIGGER IF EXISTS pricing_updated_at ON model_pricing;
DROP FUNCTION IF EXISTS update_pricing_timestamp();
DROP TABLE IF EXISTS model_pricing CASCADE;
