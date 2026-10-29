-- Whole-server recovery runs: the latest report must survive a control plane restart mid-recovery.
CREATE TABLE rehydrate_runs (
    id          TEXT PRIMARY KEY,
    state       TEXT NOT NULL,
    started_at  TEXT NOT NULL,
    finished_at TEXT NOT NULL DEFAULT '',
    report      TEXT NOT NULL
);
CREATE INDEX idx_rehydrate_runs_started ON rehydrate_runs (started_at);
