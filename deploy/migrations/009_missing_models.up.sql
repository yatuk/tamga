-- Migration: 009_missing_models.up.sql
--
-- Adds model pricing entries that were in the hardcoded providerCatalog()
-- but missing from the 008 seed data. Idempotent via ON CONFLICT DO NOTHING.

INSERT INTO model_pricing
    (provider, model_family, model_version, input_per_1k, output_per_1k, source)
VALUES
    -- OpenAI — gpt-4.1 (missing from 008)
    ('openai',    'gpt-4.1',   '2025-04-15',          0.002000, 0.008000, 'openai-pricing-page'),

    -- Anthropic — claude-3-5-haiku (missing from 008)
    ('anthropic', 'claude-3-5','haiku-20250101',       0.000800, 0.004000, 'anthropic-pricing-page'),

    -- Google — gemini-2.0-flash (missing from 008)
    ('google',    'gemini-2.0', 'flash',               0.000100, 0.000400, 'google-vertex-pricing'),

    -- Mistral (missing from 008)
    ('mistral',   'mistral',   'large',                0.002000, 0.006000, 'mistral-pricing-page'),
    ('mistral',   'mistral',   'small',                0.000200, 0.000600, 'mistral-pricing-page'),

    -- AWS Bedrock (missing from 008)
    ('bedrock',   'claude-3-5', 'sonnet-v2',           0.003000, 0.015000, 'aws-bedrock-pricing'),
    ('bedrock',   'llama-3.1',  '70b',                 0.000990, 0.000990, 'aws-bedrock-pricing'),

    -- Local / self-hosted — qwen (missing from 008)
    ('local',     'qwen-2.5',  '14b',                  0.000000, 0.000000, 'manual')
ON CONFLICT DO NOTHING;
