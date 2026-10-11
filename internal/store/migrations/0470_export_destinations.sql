-- Operator-configured outbound data export destinations (logs, audit log,
-- metrics). Secrets (headers, HMAC key) live in internal/secrets under
-- "export-destination/<id>", never in this table.
CREATE TABLE export_destinations (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    kind       TEXT NOT NULL,
    endpoint   TEXT NOT NULL DEFAULT '',
    enabled    INTEGER NOT NULL DEFAULT 1,
    apps       TEXT NOT NULL DEFAULT '[]',
    min_level  TEXT NOT NULL DEFAULT '',
    options    TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
