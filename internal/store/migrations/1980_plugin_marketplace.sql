CREATE TABLE IF NOT EXISTS plugin_marketplace_sources (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    url             TEXT NOT NULL,
    root_public_key TEXT NOT NULL,
    added_at        TEXT NOT NULL,
    added_by        TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS plugin_marketplace_state (
    source_id         TEXT PRIMARY KEY,
    last_sequence     INTEGER NOT NULL DEFAULT 0,
    last_generated_at TEXT NOT NULL DEFAULT '',
    last_hash         TEXT NOT NULL DEFAULT '',
    etag              TEXT NOT NULL DEFAULT '',
    body              TEXT NOT NULL DEFAULT '',
    fetched_at        TEXT NOT NULL DEFAULT '',
    last_error        TEXT NOT NULL DEFAULT '',
    error_kind        TEXT NOT NULL DEFAULT ''
);
