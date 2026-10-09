-- Per-database status of copying live data in from a source database.
-- Source credentials are never stored, only where the data came from.
CREATE TABLE IF NOT EXISTS database_data_imports (
    database_name TEXT PRIMARY KEY,
    status        TEXT NOT NULL,
    reason        TEXT NOT NULL DEFAULT '',
    source_host   TEXT NOT NULL DEFAULT '',
    source_port   INTEGER NOT NULL DEFAULT 0,
    source_db     TEXT NOT NULL DEFAULT '',
    checked       INTEGER NOT NULL DEFAULT 0,
    mismatched    INTEGER NOT NULL DEFAULT 0,
    detail        TEXT NOT NULL DEFAULT '',
    started_at    TEXT NOT NULL,
    finished_at   TEXT NOT NULL DEFAULT '',
    updated_at    TEXT NOT NULL
);
