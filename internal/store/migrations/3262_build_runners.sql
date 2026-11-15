-- Build runners: why a build went where it did, and an optional per app runner preference.
ALTER TABLE build_runs ADD COLUMN runner_reason TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS app_build_runner (
    app_name   TEXT PRIMARY KEY,
    node_id    TEXT NOT NULL,
    pinned     INTEGER NOT NULL DEFAULT 0,
    labels     TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);
