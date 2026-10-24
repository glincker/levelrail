-- History of every cleanup run, scheduled or manual, including dry runs.
-- The table prunes itself after a year; rows hold counts only, never row contents.
CREATE TABLE cleanup_runs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    target       TEXT NOT NULL,
    started_at   TEXT NOT NULL,
    finished_at  TEXT NOT NULL,
    days         INTEGER NOT NULL DEFAULT 0,
    dry_run      INTEGER NOT NULL DEFAULT 0,
    rows_removed INTEGER NOT NULL DEFAULT 0,
    bytes        INTEGER NOT NULL DEFAULT 0,
    trigger_kind TEXT NOT NULL,
    actor        TEXT NOT NULL DEFAULT '',
    error        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX cleanup_runs_started ON cleanup_runs (started_at);
CREATE INDEX cleanup_runs_target_started ON cleanup_runs (target, started_at);
