-- Model pricing reference table
-- Migration: 008_model_pricing.up.sql
--
-- Stores per-1K-token rates for each model+provider combination.
-- Supports multi-currency, effective date range for historical tracking,
-- and quarterly pricing updates without data loss.

CREATE TABLE IF NOT EXISTS model_pricing (
    id              SERIAL PRIMARY KEY,
    provider        TEXT NOT NULL,
    model_family    TEXT NOT NULL,
    model_version   TEXT NOT NULL,
    input_per_1k    NUMERIC(10, 6) NOT NULL,
    output_per_1k   NUMERIC(10, 6) NOT NULL,
    currency        CHAR(3) NOT NULL DEFAULT 'USD',
    effective_from  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    effective_to    TIMESTAMPTZ,
    source          TEXT,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Only one active price per model at a time
CREATE UNIQUE INDEX IF NOT EXISTS idx_model_pricing_active
ON model_pricing (provider, model_family, model_version)
WHERE effective_to IS NULL;

-- Fast lookup by provider + family + version, most recent first
CREATE INDEX IF NOT EXISTS idx_model_pricing_lookup
ON model_pricing (provider, model_family, model_version, effective_from DESC);

-- Updated_at trigger
CREATE OR REPLACE FUNCTION update_pricing_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'pricing_updated_at'
    ) THEN
        CREATE TRIGGER pricing_updated_at
        BEFORE UPDATE ON model_pricing
        FOR EACH ROW EXECUTE FUNCTION update_pricing_timestamp();
    END IF;
END $$;

-- ── Seed data (as of 2026-06-12) ──────────────────────────────────────

INSERT INTO model_pricing
    (provider, model_family, model_version, input_per_1k, output_per_1k, source)
VALUES
    -- Anthropic
    ('anthropic', 'claude-3',   'haiku-20240307',      0.000250, 0.001250, 'anthropic-pricing-page'),
    ('anthropic', 'claude-3',   'sonnet-20240229',     0.003000, 0.015000, 'anthropic-pricing-page'),
    ('anthropic', 'claude-3',   'opus-20240229',       0.015000, 0.075000, 'anthropic-pricing-page'),
    ('anthropic', 'claude-3-5', 'sonnet-20241022',     0.003000, 0.015000, 'anthropic-pricing-page'),

    -- OpenAI
    ('openai',    'gpt-4o',     '2024-08-06',          0.002500, 0.010000, 'openai-pricing-page'),
    ('openai',    'gpt-4o',     'mini-2024-07-18',     0.000150, 0.000600, 'openai-pricing-page'),
    ('openai',    'gpt-4-turbo','2024-04-09',          0.010000, 0.030000, 'openai-pricing-page'),

    -- Google Vertex AI
    ('google',    'gemini-1.5', 'pro',                 0.001250, 0.005000, 'google-vertex-pricing'),
    ('google',    'gemini-1.5', 'flash',               0.000075, 0.000300, 'google-vertex-pricing'),

    -- Local / self-hosted (infrastructure cost only)
    ('local',     'llama-3',    '8b',                  0.000000, 0.000000, 'manual'),
    ('local',     'llama-3',    '70b',                 0.000000, 0.000000, 'manual'),
    ('local',     'mistral',    '7b',                  0.000000, 0.000000, 'manual')
ON CONFLICT DO NOTHING;
