-- Key-level history of an app's environment variable edits. Values are never stored.
CREATE TABLE env_changes (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    app_name TEXT NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE,
    at       TEXT NOT NULL,
    actor    TEXT NOT NULL DEFAULT '',
    key      TEXT NOT NULL,
    action   TEXT NOT NULL,
    source   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_env_changes_app ON env_changes (app_name, id DESC);
