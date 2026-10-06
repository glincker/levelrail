-- Global environments (dev, test, uat, production, plus custom and dynamic
-- preview ones) live under one reserved built-in project, so no table rebuild
-- is needed: an app's environment_id was always independent of its project_id.
ALTER TABLE environments ADD COLUMN kind TEXT NOT NULL DEFAULT 'custom'
    CHECK (kind IN ('dev', 'test', 'uat', 'production', 'preview', 'custom'));
ALTER TABLE environments ADD COLUMN scope TEXT NOT NULL DEFAULT 'project'
    CHECK (scope IN ('project', 'global'));
ALTER TABLE environments ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

UPDATE environments SET kind = 'preview' WHERE id LIKE 'preview-env-%' OR name = 'Preview';

INSERT OR IGNORE INTO projects (id, name) VALUES ('proj_global', 'Instance');

INSERT OR IGNORE INTO environments (id, project_id, name, kind, scope, protected, sort_order) VALUES
    ('env_dev', 'proj_global', 'Development', 'dev', 'global', 0, 10),
    ('env_test', 'proj_global', 'Test', 'test', 'global', 0, 20),
    ('env_uat', 'proj_global', 'UAT', 'uat', 'global', 0, 30),
    ('env_production', 'proj_global', 'Production', 'production', 'global', 1, 40);

ALTER TABLE desired_databases ADD COLUMN environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL;

CREATE INDEX idx_desired_databases_environment_id ON desired_databases (environment_id);
CREATE INDEX idx_environments_kind ON environments (kind);
