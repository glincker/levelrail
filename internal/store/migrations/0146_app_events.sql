-- Config and lifecycle events per app (restart, env/secret/config changes,
-- scale, suspend, resume, freeze override). Key names only, never values.
CREATE TABLE IF NOT EXISTS app_events (
    id           TEXT PRIMARY KEY,
    app_name     TEXT NOT NULL,
    kind         TEXT NOT NULL,
    actor        TEXT NOT NULL DEFAULT '',
    title        TEXT NOT NULL DEFAULT '',
    detail       TEXT NOT NULL DEFAULT '',
    keys         TEXT NOT NULL DEFAULT '[]',
    created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_app_events_app_time ON app_events (app_name, created_at DESC, id DESC);
