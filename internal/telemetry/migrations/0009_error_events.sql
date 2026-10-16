-- Error tracking: one issue per fingerprint with a bounded ring of sample
-- events. resource_id is the app the ingest key belongs to ("service:<name>").
CREATE TABLE error_issues (
    resource_id    TEXT NOT NULL,
    fingerprint    TEXT NOT NULL,
    title          TEXT NOT NULL,
    level          TEXT NOT NULL DEFAULT 'error',
    first_seen_ns  INTEGER NOT NULL,
    last_seen_ns   INTEGER NOT NULL,
    count          INTEGER NOT NULL DEFAULT 0,
    last_release   TEXT NOT NULL DEFAULT '',
    last_deploy_id TEXT,
    status         TEXT NOT NULL DEFAULT 'open',
    regressed      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (resource_id, fingerprint)
) WITHOUT ROWID;

CREATE INDEX idx_error_issues_last ON error_issues (resource_id, last_seen_ns);

CREATE TABLE error_events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    resource_id TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    event_id    TEXT NOT NULL DEFAULT '',
    ts_ns       INTEGER NOT NULL,
    release     TEXT NOT NULL DEFAULT '',
    environment TEXT NOT NULL DEFAULT '',
    deploy_id   TEXT,
    raw         TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_error_events_issue ON error_events (resource_id, fingerprint, id);
CREATE INDEX idx_error_events_ts ON error_events (ts_ns);
