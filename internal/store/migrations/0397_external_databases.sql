-- Databases this platform connects to but does not run. The password is
-- never stored here: it lives in the envelope-encrypted secrets store.
CREATE TABLE IF NOT EXISTS external_databases (
    name             TEXT PRIMARY KEY,
    engine           TEXT NOT NULL,
    host             TEXT NOT NULL,
    port             INTEGER NOT NULL,
    username         TEXT NOT NULL DEFAULT '',
    database_name    TEXT NOT NULL DEFAULT '',
    tls_mode         TEXT NOT NULL DEFAULT '',
    network          TEXT NOT NULL DEFAULT '',
    node_id          TEXT NOT NULL DEFAULT '',
    project_id       TEXT,
    source_container TEXT NOT NULL DEFAULT '',
    health_status    TEXT NOT NULL DEFAULT '',
    health_reason    TEXT NOT NULL DEFAULT '',
    health_latency_ms INTEGER NOT NULL DEFAULT 0,
    health_checked_at TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);
