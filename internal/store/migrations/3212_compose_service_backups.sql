-- Scheduled dumps of database services inside compose apps. Own tables so the
-- managed-database and volume backup history tables stay untouched.
CREATE TABLE compose_service_backups (
    app                TEXT NOT NULL,
    service            TEXT NOT NULL,
    engine             TEXT NOT NULL,
    backup_target_id   TEXT REFERENCES backup_targets(id) ON DELETE SET NULL,
    backup_schedule    TEXT NOT NULL DEFAULT '',
    backup_retain      INTEGER NOT NULL DEFAULT 0,
    backup_retain_days INTEGER NOT NULL DEFAULT 0,
    enabled            INTEGER NOT NULL DEFAULT 1,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL,
    PRIMARY KEY (app, service)
);

CREATE INDEX idx_compose_service_backups_schedule ON compose_service_backups (backup_schedule) WHERE backup_schedule != '' AND enabled = 1;

-- target_id has no foreign key on purpose: a run row must outlive its target so the object it points at stays traceable.
CREATE TABLE compose_service_backup_runs (
    id          TEXT PRIMARY KEY,
    app         TEXT NOT NULL,
    service     TEXT NOT NULL,
    target_id   TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    object_key  TEXT NOT NULL DEFAULT '',
    size_bytes  INTEGER NOT NULL DEFAULT 0,
    checksum    TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    started_at  TEXT NOT NULL,
    finished_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_compose_service_backup_runs_service ON compose_service_backup_runs (app, service, started_at DESC);
