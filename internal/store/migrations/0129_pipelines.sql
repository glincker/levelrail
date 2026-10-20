-- One row per saved pipeline definition (YAML text). source is 'ui' or
-- 'repo' (synced from a pipeline directory in the app's repository).
CREATE TABLE pipelines (
    id TEXT PRIMARY KEY,
    app_name TEXT NOT NULL,
    name TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'ui',
    yaml TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (app_name, name)
);

CREATE INDEX idx_pipelines_app ON pipelines (app_name);
