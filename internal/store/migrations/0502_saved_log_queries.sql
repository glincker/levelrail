-- Named log searches an operator saves, optionally scoped to one app,
-- visible to just the owner ('private') or to everyone who can see the app
-- ('instance'). pinned puts the query at the top of the app's log page.
CREATE TABLE saved_log_queries (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    app        TEXT NOT NULL DEFAULT '',
    spec_json  TEXT NOT NULL,
    owner_type TEXT NOT NULL,
    owner_id   TEXT NOT NULL,
    owner_name TEXT NOT NULL DEFAULT '',
    visibility TEXT NOT NULL DEFAULT 'private',
    pinned     INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_saved_log_queries_owner_name ON saved_log_queries (owner_id, app, name);
CREATE INDEX idx_saved_log_queries_app ON saved_log_queries (app);
