-- One row per archive attempt (scheduled or manual), kept for the UI history.
CREATE TABLE log_archive_runs (
    id          TEXT PRIMARY KEY,
    policy_id   TEXT NOT NULL DEFAULT '',
    app_name    TEXT NOT NULL DEFAULT '',
    target_id   TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('scheduled', 'manual')),
    from_ns     INTEGER NOT NULL,
    to_ns       INTEGER NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    objects     INTEGER NOT NULL DEFAULT 0,
    lines       INTEGER NOT NULL DEFAULT 0,
    bytes       INTEGER NOT NULL DEFAULT 0,
    error       TEXT NOT NULL DEFAULT '',
    started_at  TEXT NOT NULL,
    finished_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_log_archive_runs_app ON log_archive_runs (app_name, started_at DESC);
