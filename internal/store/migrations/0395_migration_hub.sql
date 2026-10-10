-- Server-level migration hub: one session per source server, one item per
-- source database. The source password is never stored.
CREATE TABLE IF NOT EXISTS migration_hub_sessions (
    id             TEXT PRIMARY KEY,
    engine         TEXT NOT NULL,
    host           TEXT NOT NULL,
    port           INTEGER NOT NULL DEFAULT 0,
    user_name      TEXT NOT NULL DEFAULT '',
    tls            INTEGER NOT NULL DEFAULT 0,
    node_id        TEXT NOT NULL DEFAULT '',
    helper_network TEXT NOT NULL DEFAULT '',
    source_container TEXT NOT NULL DEFAULT '',
    server_version TEXT NOT NULL DEFAULT '',
    server_major   INTEGER NOT NULL DEFAULT 0,
    free_bytes     INTEGER NOT NULL DEFAULT -1,
    step           TEXT NOT NULL,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS migration_hub_items (
    session_id     TEXT NOT NULL REFERENCES migration_hub_sessions(id) ON DELETE CASCADE,
    source_db      TEXT NOT NULL,
    size_bytes     INTEGER NOT NULL DEFAULT 0,
    table_count    INTEGER NOT NULL DEFAULT 0,
    extensions     TEXT NOT NULL DEFAULT '',
    target_name    TEXT NOT NULL,
    target_version TEXT NOT NULL DEFAULT '',
    selected       INTEGER NOT NULL DEFAULT 0,
    status         TEXT NOT NULL DEFAULT 'pending',
    reason         TEXT NOT NULL DEFAULT '',
    created_target INTEGER NOT NULL DEFAULT 0,
    checked        INTEGER NOT NULL DEFAULT 0,
    mismatched     INTEGER NOT NULL DEFAULT 0,
    detail         TEXT NOT NULL DEFAULT '',
    started_at     TEXT NOT NULL DEFAULT '',
    finished_at    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (session_id, source_db)
);
