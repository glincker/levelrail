CREATE TABLE IF NOT EXISTS build_runs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    app_name      TEXT NOT NULL,
    tag           TEXT NOT NULL DEFAULT '',
    started_at    TEXT NOT NULL,
    duration_ms   INTEGER NOT NULL,
    queue_wait_ms INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL,
    cached_steps  INTEGER NOT NULL DEFAULT 0,
    total_steps   INTEGER NOT NULL DEFAULT 0,
    error         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_build_runs_app ON build_runs (app_name, id);

CREATE TABLE IF NOT EXISTS build_limit_settings (
    key        TEXT PRIMARY KEY,
    value      INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL DEFAULT ''
);
