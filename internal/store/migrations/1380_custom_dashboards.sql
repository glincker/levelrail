CREATE TABLE custom_dashboards (
    id         TEXT PRIMARY KEY,
    owner      TEXT NOT NULL,
    scope      TEXT NOT NULL CHECK (scope IN ('user', 'shared')),
    project_id TEXT NOT NULL DEFAULT '',
    is_default INTEGER NOT NULL DEFAULT 0,
    name       TEXT NOT NULL,
    body       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_custom_dashboards_name
    ON custom_dashboards (CASE WHEN scope = 'shared' THEN 'shared' ELSE owner END, name);

CREATE INDEX idx_custom_dashboards_project ON custom_dashboards (project_id, is_default);
