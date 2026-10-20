-- A backup target is an S3-compatible bucket (AWS S3, Cloudflare R2, or
-- any other S3-compatible endpoint) a user connects as a destination for
-- database backups. Deliberately no access_key_id/secret_access_key
-- columns here, the same reasoning migrations/0003_desired_databases.sql
-- already established for database passwords: credentials go through
-- internal/secrets' envelope encryption, keyed by "backup-target/<id>",
-- never as plaintext (or even encrypted-in-the-same-row) columns in this
-- table. Losing this table to a read-only replica leak or a backup of
-- the control plane's own SQLite file should never hand over live bucket
-- credentials.
--
-- backup_history is a append-only record of attempted backups, kept
-- regardless of success: a failed attempt is exactly the row an operator
-- most needs to see, matching the project's "never silently swallow a
-- failure" standard applied everywhere else (deploy conditions, reconcile
-- results).
CREATE TABLE backup_targets (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    provider   TEXT NOT NULL CHECK (provider IN ('aws', 'r2', 'custom')),
    endpoint   TEXT NOT NULL DEFAULT '',
    region     TEXT NOT NULL DEFAULT '',
    bucket     TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE backup_history (
    id            TEXT PRIMARY KEY,
    database_name TEXT NOT NULL,
    target_id     TEXT NOT NULL REFERENCES backup_targets(id),
    object_key    TEXT NOT NULL,
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    error         TEXT NOT NULL DEFAULT '',
    started_at    TEXT NOT NULL,
    finished_at   TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_backup_history_database_name ON backup_history(database_name, started_at DESC);
