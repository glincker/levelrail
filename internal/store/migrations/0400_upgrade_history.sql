CREATE TABLE IF NOT EXISTS upgrade_history (
    seq           INTEGER PRIMARY KEY AUTOINCREMENT,
    id            TEXT NOT NULL UNIQUE,
    kind          TEXT NOT NULL,
    from_version  TEXT NOT NULL DEFAULT '',
    to_version    TEXT NOT NULL,
    channel       TEXT NOT NULL DEFAULT '',
    schema_before INTEGER NOT NULL DEFAULT -1,
    schema_after  INTEGER NOT NULL DEFAULT -1,
    occurred_at   TEXT NOT NULL,
    initiator     TEXT NOT NULL DEFAULT 'unknown',
    method        TEXT NOT NULL DEFAULT '',
    backup_name   TEXT NOT NULL DEFAULT '',
    health        TEXT NOT NULL DEFAULT 'booted',
    notes         TEXT NOT NULL DEFAULT '',
    notes_state   TEXT NOT NULL DEFAULT 'pending',
    acked_by_type TEXT,
    acked_by_id   TEXT,
    acked_by_name TEXT,
    acked_at      TEXT
);

CREATE TRIGGER IF NOT EXISTS upgrade_history_no_delete
BEFORE DELETE ON upgrade_history
BEGIN
    SELECT RAISE(ABORT, 'upgrade history is append-only');
END;

CREATE TRIGGER IF NOT EXISTS upgrade_history_immutable
BEFORE UPDATE OF seq, id, kind, from_version, to_version, channel, schema_before, schema_after,
    occurred_at, initiator, method, backup_name, health ON upgrade_history
BEGIN
    SELECT RAISE(ABORT, 'upgrade history is append-only');
END;

CREATE TRIGGER IF NOT EXISTS upgrade_history_notes_once
BEFORE UPDATE OF notes, notes_state ON upgrade_history
WHEN OLD.notes_state <> 'pending'
BEGIN
    SELECT RAISE(ABORT, 'upgrade history is append-only');
END;

CREATE TRIGGER IF NOT EXISTS upgrade_history_ack_once
BEFORE UPDATE OF acked_by_type, acked_by_id, acked_by_name, acked_at ON upgrade_history
WHEN OLD.acked_at IS NOT NULL
BEGIN
    SELECT RAISE(ABORT, 'upgrade history is append-only');
END;
