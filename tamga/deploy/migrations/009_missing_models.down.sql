-- Migration: 009_missing_models.down.sql
--
-- Removes the pricing entries added in the up migration.
-- Only removes entries that are still active (effective_to IS NULL)
-- from the specific source URLs used in the up migration.

DELETE FROM model_pricing
WHERE effective_to IS NULL
  AND source IN (
    'openai-pricing-page',
    'anthropic-pricing-page',
    'google-vertex-pricing',
    'mistral-pricing-page',
    'aws-bedrock-pricing',
    'manual'
  )
  AND (provider, model_family, model_version) IN (
    ('openai',    'gpt-4.1',   '2025-04-15'),
    ('anthropic', 'claude-3-5','haiku-20250101'),
    ('google',    'gemini-2.0', 'flash'),
    ('mistral',   'mistral',   'large'),
    ('mistral',   'mistral',   'small'),
    ('bedrock',   'claude-3-5', 'sonnet-v2'),
    ('bedrock',   'llama-3.1',  '70b'),
    ('local',     'qwen-2.5',  '14b')
  );
