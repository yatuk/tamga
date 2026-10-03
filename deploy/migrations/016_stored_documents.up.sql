-- 016_stored_documents: custom patterns, team roles and webhooks, kept across
-- restarts. Each record is one JSON document; webhook documents are
-- AES-256-GCM encrypted with TAMGA_VAULT_KEY (encrypted = true), because a
-- webhook URL and its token are credentials.
-- The proxy also creates this table at startup (docstore.Schema).

CREATE TABLE IF NOT EXISTS stored_documents (
    kind       TEXT NOT NULL,
    id         TEXT NOT NULL,
    doc        TEXT NOT NULL,
    encrypted  BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, id)
);
