-- Organizations group projects (mirrors 0022_projects.sql's own
-- opaque-id, nullable-label pattern). Environments belong to a project
-- and optionally tag a service, the same nullable-FK shape 0022 already
-- established for project_id on desired_services.

CREATE TABLE organizations (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

ALTER TABLE projects ADD COLUMN org_id TEXT REFERENCES organizations(id) ON DELETE SET NULL;

CREATE INDEX idx_projects_org_id ON projects (org_id);

CREATE TABLE environments (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_environments_project_id ON environments (project_id);

ALTER TABLE desired_services ADD COLUMN environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL;

CREATE INDEX idx_desired_services_environment_id ON desired_services (environment_id);
