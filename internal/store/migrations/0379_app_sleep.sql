-- Per-app opt-in to stop after idle_minutes without requests and wake on the
-- next one. since is the activity baseline (enable or last wake time).
CREATE TABLE IF NOT EXISTS app_sleep (
    service_name TEXT PRIMARY KEY,
    idle_minutes INTEGER NOT NULL DEFAULT 0,
    sleeping     INTEGER NOT NULL DEFAULT 0,
    since        TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
