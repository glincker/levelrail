-- Delivery state per export destination: health counters plus the audit
-- export cursor (created_at, id of the last row delivered) that makes the
-- audit stream restart-safe with at-least-once semantics.
CREATE TABLE export_destination_status (
    destination_id  TEXT PRIMARY KEY REFERENCES export_destinations(id) ON DELETE CASCADE,
    last_success_at TEXT NOT NULL DEFAULT '',
    last_error      TEXT NOT NULL DEFAULT '',
    last_error_at   TEXT NOT NULL DEFAULT '',
    sent_total      INTEGER NOT NULL DEFAULT 0,
    dropped_total   INTEGER NOT NULL DEFAULT 0,
    failures_total  INTEGER NOT NULL DEFAULT 0,
    queue_depth     INTEGER NOT NULL DEFAULT 0,
    circuit_open    INTEGER NOT NULL DEFAULT 0,
    cursor_at       TEXT NOT NULL DEFAULT '',
    cursor_id       TEXT NOT NULL DEFAULT '',
    updated_at      TEXT NOT NULL
);
