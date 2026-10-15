-- Audit log streaming sinks. Credentials live in internal/secrets under
-- "audit-sink/<id>", never in these tables. cursor_seq is the last
-- audit_log.seq the sink acknowledged, advanced only after a delivery.
CREATE TABLE audit_sinks (
    id                   TEXT PRIMARY KEY,
    name                 TEXT NOT NULL UNIQUE,
    kind                 TEXT NOT NULL,
    enabled              INTEGER NOT NULL DEFAULT 1,
    config               TEXT NOT NULL DEFAULT '{}',
    cursor_seq           INTEGER NOT NULL DEFAULT 0,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    next_attempt_at      TEXT NOT NULL DEFAULT '',
    last_error           TEXT NOT NULL DEFAULT '',
    last_delivered_at    TEXT NOT NULL DEFAULT '',
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);

CREATE TABLE audit_sink_deliveries (
    id         TEXT PRIMARY KEY,
    sink_id    TEXT NOT NULL REFERENCES audit_sinks(id) ON DELETE CASCADE,
    from_seq   INTEGER NOT NULL,
    to_seq     INTEGER NOT NULL,
    entries    INTEGER NOT NULL,
    state      TEXT NOT NULL,
    attempt    INTEGER NOT NULL,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX audit_sink_deliveries_sink ON audit_sink_deliveries (sink_id, created_at DESC);
