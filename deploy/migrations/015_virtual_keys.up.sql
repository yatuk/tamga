-- 015_virtual_keys: Tamga's own keys, kept across restarts.
-- A key is stored as a SHA-256 hash and a short prefix; the value itself is
-- shown once at creation and never stored. org_id, role and user_id are the
-- identity a request made with the key has on the proxy path.
-- The proxy also creates this table at startup (apikeys.Schema), so a
-- database whose init scripts already ran needs no manual step.

CREATE TABLE IF NOT EXISTS virtual_keys (
    id           TEXT PRIMARY KEY,
    key_hash     TEXT NOT NULL UNIQUE,
    prefix       TEXT NOT NULL,
    label        TEXT NOT NULL DEFAULT '',
    scope        TEXT NOT NULL,
    org_id       TEXT NOT NULL DEFAULT '',
    role         TEXT NOT NULL DEFAULT '',
    user_id      TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_virtual_keys_active ON virtual_keys(created_at DESC) WHERE revoked_at IS NULL;
