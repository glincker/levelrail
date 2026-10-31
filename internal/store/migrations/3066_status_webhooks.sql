-- Signed webhook subscribers for the public status page, plus the delivery outbox and log.
CREATE TABLE status_webhooks (
    id                   TEXT PRIMARY KEY,
    url                  TEXT NOT NULL,
    label                TEXT NOT NULL DEFAULT '',
    events               TEXT NOT NULL DEFAULT '',
    secret_plain         TEXT NOT NULL DEFAULT '',
    sealed               INTEGER NOT NULL DEFAULT 0,
    paused               INTEGER NOT NULL DEFAULT 0,
    paused_reason        TEXT NOT NULL DEFAULT '',
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);

CREATE TABLE status_webhook_deliveries (
    id              TEXT PRIMARY KEY,
    webhook_id      TEXT NOT NULL,
    event_id        TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    payload         TEXT NOT NULL,
    state           TEXT NOT NULL,
    attempts        INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT NOT NULL,
    last_status     INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    delivered_at    TEXT,
    UNIQUE (webhook_id, event_id)
);
CREATE INDEX status_webhook_deliveries_due ON status_webhook_deliveries (state, next_attempt_at);
