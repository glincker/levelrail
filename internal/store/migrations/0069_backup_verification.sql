-- An unverified backup is not actually a backup: this adds a checksum
-- recorded at backup time (backup_history.checksum_sha256) plus an
-- append-only backup_verification table, the verification counterpart of
-- backup_history itself, recording each attempt to re-download a stored
-- backup and confirm it is still intact.

ALTER TABLE backup_history ADD COLUMN checksum_sha256 TEXT NOT NULL DEFAULT '';

CREATE TABLE backup_verification (
    id                TEXT PRIMARY KEY,
    backup_history_id TEXT NOT NULL REFERENCES backup_history(id),
    status            TEXT NOT NULL CHECK (status IN ('running', 'passed', 'failed')),
    checksum_match    INTEGER NOT NULL DEFAULT 0,
    size_match        INTEGER NOT NULL DEFAULT 0,
    format_valid      INTEGER NOT NULL DEFAULT 0,
    downloaded_bytes  INTEGER NOT NULL DEFAULT 0,
    error             TEXT NOT NULL DEFAULT '',
    checked_by        TEXT NOT NULL DEFAULT '',
    started_at        TEXT NOT NULL,
    finished_at       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_backup_verification_backup_history_id ON backup_verification(backup_history_id, started_at DESC);
