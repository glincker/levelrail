CREATE TABLE inbox_read_state (
    principal   TEXT PRIMARY KEY,
    read_before TEXT NOT NULL
);

CREATE TABLE inbox_read (
    principal  TEXT NOT NULL,
    history_id TEXT NOT NULL,
    PRIMARY KEY (principal, history_id)
);

CREATE TABLE inbox_prefs (
    principal         TEXT PRIMARY KEY,
    min_severity      TEXT NOT NULL DEFAULT 'warning',
    muted_apps        TEXT NOT NULL DEFAULT '',
    digest_enabled    INTEGER NOT NULL DEFAULT 0,
    digest_hour       INTEGER NOT NULL DEFAULT 8,
    digest_channel_id TEXT NOT NULL DEFAULT '',
    last_digest_at    TEXT NOT NULL DEFAULT '',
    updated_at        TEXT NOT NULL
);
