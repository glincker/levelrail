-- Guided app import from another platform. The source token is never
-- stored. entry_json holds the non-secret inventory row of one source app.
CREATE TABLE IF NOT EXISTS app_import_sessions (
    id            TEXT PRIMARY KEY,
    platform      TEXT NOT NULL,
    source_url    TEXT NOT NULL,
    step          TEXT NOT NULL,
    collision     TEXT NOT NULL DEFAULT 'suffix',
    mappings_json TEXT NOT NULL DEFAULT '[]',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS app_import_items (
    session_id     TEXT NOT NULL REFERENCES app_import_sessions(id) ON DELETE CASCADE,
    source_id      TEXT NOT NULL,
    source_name    TEXT NOT NULL,
    target_name    TEXT NOT NULL DEFAULT '',
    kind           TEXT NOT NULL DEFAULT 'app',
    state          TEXT NOT NULL DEFAULT 'planned',
    selected       INTEGER NOT NULL DEFAULT 1,
    reason         TEXT NOT NULL DEFAULT '',
    entry_json     TEXT NOT NULL DEFAULT '{}',
    domains_json   TEXT NOT NULL DEFAULT '[]',
    volumes_json   TEXT NOT NULL DEFAULT '[]',
    attempt_id     TEXT NOT NULL DEFAULT '',
    created_target INTEGER NOT NULL DEFAULT 0,
    updated_at     TEXT NOT NULL,
    PRIMARY KEY (session_id, source_id)
);
