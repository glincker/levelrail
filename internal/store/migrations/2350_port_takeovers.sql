-- Same-server port takeover runs: the previous listen addresses and settings are kept so a rollback restores them. No secrets.
CREATE TABLE port_takeovers (
    id             TEXT PRIMARY KEY,
    state          TEXT NOT NULL CHECK (state IN ('preparing', 'applying', 'verifying', 'done', 'rolling_back', 'rolled_back', 'failed')),
    actor          TEXT NOT NULL DEFAULT '',
    previous_json  TEXT NOT NULL DEFAULT '{}',
    target_http    TEXT NOT NULL,
    target_https   TEXT NOT NULL,
    steps_json     TEXT NOT NULL DEFAULT '[]',
    verify_json    TEXT NOT NULL DEFAULT '[]',
    error          TEXT NOT NULL DEFAULT '',
    boots          INTEGER NOT NULL DEFAULT 0,
    window_seconds INTEGER NOT NULL DEFAULT 300,
    deadline_at    TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    finished_at    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX port_takeovers_created ON port_takeovers (created_at DESC);
