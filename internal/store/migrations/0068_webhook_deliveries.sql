CREATE TABLE webhook_deliveries (
    id                TEXT PRIMARY KEY,
    service_name      TEXT NOT NULL,
    provider          TEXT NOT NULL,
    event_type        TEXT NOT NULL,
    header_fields     TEXT NOT NULL DEFAULT '{}',
    signature_valid   INTEGER NOT NULL DEFAULT 0,
    matched           INTEGER NOT NULL DEFAULT 0,
    status_code       INTEGER NOT NULL DEFAULT 0,
    payload           BLOB NOT NULL DEFAULT '',
    payload_truncated INTEGER NOT NULL DEFAULT 0,
    error             TEXT NOT NULL DEFAULT '',
    received_at       TEXT NOT NULL
);

CREATE INDEX idx_webhook_deliveries_service_name ON webhook_deliveries(service_name);
