-- Stage 1 of multi-service apps: an app can own N desired_services rows
-- (a "web" plus a "worker"), read path and schema only, no fan-out or
-- reconciler change yet. ON DELETE CASCADE (unlike project_id's SET
-- NULL) because an app owns its services' lifecycle: deleting the app
-- must delete its services, not orphan them.
CREATE TABLE apps (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

ALTER TABLE desired_services ADD COLUMN app_id TEXT REFERENCES apps(id) ON DELETE CASCADE;

CREATE INDEX idx_desired_services_app_id ON desired_services (app_id);

-- Backfill: every existing service becomes its own single-service app,
-- id/name equal to the service's own name, preserving today's 1:1
-- behavior exactly.
INSERT INTO apps (id, name, project_id, created_at, updated_at)
SELECT name, name, project_id, updated_at, updated_at FROM desired_services;

UPDATE desired_services SET app_id = name;
