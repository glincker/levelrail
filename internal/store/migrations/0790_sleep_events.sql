CREATE TABLE IF NOT EXISTS sleep_events (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    service_name     TEXT NOT NULL,
    kind             TEXT NOT NULL,
    at               TEXT NOT NULL,
    reason           TEXT NOT NULL DEFAULT '',
    idle_minutes     INTEGER NOT NULL DEFAULT 0,
    trigger_path     TEXT NOT NULL DEFAULT '',
    trigger_source   TEXT NOT NULL DEFAULT '',
    detail           TEXT NOT NULL DEFAULT '',
    duration_seconds INTEGER,
    wake_latency_ms  INTEGER,
    actor            TEXT NOT NULL DEFAULT 'system'
);
CREATE INDEX IF NOT EXISTS idx_sleep_events_service_at ON sleep_events (service_name, at);
CREATE INDEX IF NOT EXISTS idx_sleep_events_at ON sleep_events (at);
