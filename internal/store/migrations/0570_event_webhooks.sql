-- Outbound event webhooks and their delivery log. The signing secret lives
-- in internal/secrets under "event-webhook/<id>", never in these tables.
CREATE TABLE event_webhooks (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    url        TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    events     TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE event_webhook_deliveries (
    id              TEXT PRIMARY KEY,
    webhook_id      TEXT NOT NULL REFERENCES event_webhooks(id) ON DELETE CASCADE,
    event_id        TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    payload         TEXT NOT NULL,
    state           TEXT NOT NULL,
    attempts        INTEGER NOT NULL DEFAULT 0,
    response_status INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT NOT NULL DEFAULT '',
    next_attempt_at TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX event_webhook_deliveries_due ON event_webhook_deliveries (state, next_attempt_at);
CREATE INDEX event_webhook_deliveries_hook ON event_webhook_deliveries (webhook_id, created_at DESC);
