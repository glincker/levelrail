-- Shared env vars at the project label level (migrations/0022_projects.sql):
-- the base layer an app's own env, secrets, and storage-target vars all
-- override on key collision, mirroring the precedence order
-- internal/reconcile/application.Controller.resolveEnv already
-- establishes for those three. Plain values only, matching SaveProject's
-- own "a project is a label, not a lifecycle owner" scope: no secret
-- support here, an app that needs a shared secret still declares it
-- through its own SecretEnv.
CREATE TABLE project_env_vars (
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    PRIMARY KEY (project_id, key)
);
