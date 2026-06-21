-- 010_event_outbox: durable event outbox for NATS JetStream delivery.
-- Events are written here synchronously (in the same DB transaction as
-- request_log) and published to NATS by a background worker. This
-- guarantees at-least-once delivery across process restarts.

CREATE TABLE IF NOT EXISTS event_outbox (
    id          BIGSERIAL PRIMARY KEY,
    event_type  TEXT        NOT NULL,
    payload     JSONB       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    error_count INT         NOT NULL DEFAULT 0,
    last_error  TEXT
);

-- Index for the background worker: only unpaginated, non-exhausted rows.
CREATE INDEX idx_outbox_pending ON event_outbox (created_at)
    WHERE published_at IS NULL AND error_count < 10;
