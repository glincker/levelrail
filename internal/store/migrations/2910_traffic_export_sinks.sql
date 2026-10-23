-- Traffic export sinks. Only non-secret settings live here; the destination
-- URL, tokens and auth headers are in internal/secrets under a per-sink namespace.
CREATE TABLE traffic_export_sinks (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('posthog', 'otlp', 'webhook')),
    enabled     INTEGER NOT NULL DEFAULT 1,
    host        TEXT NOT NULL DEFAULT '',
    target_host TEXT NOT NULL DEFAULT '',
    fields      TEXT NOT NULL DEFAULT '[]',
    ip_truncate INTEGER NOT NULL DEFAULT 1,
    sample      REAL NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE UNIQUE INDEX traffic_export_sinks_name ON traffic_export_sinks (name);
