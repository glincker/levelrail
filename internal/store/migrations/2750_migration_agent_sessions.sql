-- Read-only migration agent sessions: one row per source host inspected before a cutover. The token is stored hashed and is single use; the session never becomes a node.
CREATE TABLE migration_agent_sessions (
    id                 TEXT PRIMARY KEY,
    label              TEXT NOT NULL DEFAULT '',
    token_hash         TEXT NOT NULL,
    token_expires_at   TEXT NOT NULL,
    token_used_at      TEXT,
    state              TEXT NOT NULL DEFAULT 'pending',
    cert_fingerprint   TEXT NOT NULL DEFAULT '',
    cert_serial        TEXT NOT NULL DEFAULT '',
    session_expires_at TEXT,
    connected_at       TEXT,
    last_seen_at       TEXT,
    revoked_at         TEXT,
    agent_version      TEXT NOT NULL DEFAULT '',
    agent_os           TEXT NOT NULL DEFAULT '',
    agent_arch         TEXT NOT NULL DEFAULT '',
    inventory_json     TEXT NOT NULL DEFAULT '',
    scan_json          TEXT NOT NULL DEFAULT '',
    created_by         TEXT NOT NULL DEFAULT '',
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);
CREATE INDEX migration_agent_sessions_state ON migration_agent_sessions (state);
