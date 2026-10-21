-- One row per outbound email attempt outcome. to_hash is always stored;
-- to_display is a truncated address unless APP_EMAIL_LOG_FULL_ADDRESS is on.
CREATE TABLE email_deliveries (
    id          TEXT PRIMARY KEY,
    created_at  TEXT NOT NULL,
    provider    TEXT NOT NULL,
    to_hash     TEXT NOT NULL,
    to_display  TEXT NOT NULL,
    kind        TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL CHECK (status IN ('sent', 'failed', 'bounced')),
    message_id  TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    error_kind  TEXT NOT NULL DEFAULT '',
    attempts    INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX email_deliveries_created_idx ON email_deliveries (created_at DESC);
CREATE INDEX email_deliveries_status_idx ON email_deliveries (status, created_at DESC);
