-- Restore drills: an automatic or manual restore of a stored backup into a
-- scratch resource, validated and destroyed. One row per attempt.
CREATE TABLE backup_drills (
    id                TEXT PRIMARY KEY,
    backup_history_id TEXT NOT NULL,
    resource_kind     TEXT NOT NULL CHECK (resource_kind IN ('database', 'volume')),
    database_name     TEXT NOT NULL DEFAULT '',
    service_name      TEXT NOT NULL DEFAULT '',
    volume_name       TEXT NOT NULL DEFAULT '',
    target_id         TEXT NOT NULL DEFAULT '',
    trigger           TEXT NOT NULL DEFAULT 'manual' CHECK (trigger IN ('manual', 'scheduled')),
    status            TEXT NOT NULL CHECK (status IN ('running', 'passed', 'failed')),
    stage             TEXT NOT NULL DEFAULT '',
    object_ok         INTEGER NOT NULL DEFAULT 0,
    checksum_ok       INTEGER NOT NULL DEFAULT 0,
    restore_ok        INTEGER NOT NULL DEFAULT 0,
    content_ok        INTEGER NOT NULL DEFAULT 0,
    files             INTEGER NOT NULL DEFAULT 0,
    bytes             INTEGER NOT NULL DEFAULT 0,
    duration_ms       INTEGER NOT NULL DEFAULT 0,
    error             TEXT NOT NULL DEFAULT '',
    started_at        TEXT NOT NULL,
    finished_at       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_backup_drills_backup ON backup_drills(backup_history_id, started_at DESC);
CREATE INDEX idx_backup_drills_started ON backup_drills(started_at DESC);
