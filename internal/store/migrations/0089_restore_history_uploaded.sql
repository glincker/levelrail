-- restore_history.backup_history_id (migrations/0019) is NOT NULL, a real
-- foreign key into backup_history: every restore this schema could
-- express so far names a backup this platform itself took. Restoring
-- from an operator-uploaded dump file (never stored in backup_history at
-- all) needs that column to be genuinely absent, not a sentinel string
-- that would fail the foreign key check, so this drops the NOT NULL the
-- same way DesiredDatabase.BackupTargetID's own column already treats an
-- optional reference: SQL NULL, Go's empty string on the read side.
--
-- SQLite has no ALTER TABLE ... ALTER COLUMN, so this is the same
-- recreate-and-copy pattern migrations/0016 already uses, carrying every
-- column migrations/0019 and 0075 added forward.
CREATE TABLE restore_history_new (
    id                TEXT PRIMARY KEY,
    database_name     TEXT NOT NULL,
    resource_kind     TEXT NOT NULL DEFAULT 'database' CHECK (resource_kind IN ('database', 'volume')),
    service_name      TEXT NOT NULL DEFAULT '',
    volume_name       TEXT NOT NULL DEFAULT '',
    backup_history_id TEXT REFERENCES backup_history(id),
    status            TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    error             TEXT NOT NULL DEFAULT '',
    started_at        TEXT NOT NULL,
    finished_at       TEXT NOT NULL DEFAULT ''
);

INSERT INTO restore_history_new (id, database_name, resource_kind, service_name, volume_name, backup_history_id, status, error, started_at, finished_at)
SELECT id, database_name, resource_kind, service_name, volume_name, backup_history_id, status, error, started_at, finished_at FROM restore_history;

DROP TABLE restore_history;

ALTER TABLE restore_history_new RENAME TO restore_history;

CREATE INDEX idx_restore_history_database_name ON restore_history(database_name, started_at DESC);
CREATE INDEX idx_restore_history_service_volume ON restore_history(service_name, volume_name, started_at DESC) WHERE resource_kind = 'volume';
