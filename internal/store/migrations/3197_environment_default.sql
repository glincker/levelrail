-- One default environment per project, used when a create names a project but no environment.
ALTER TABLE environments ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX idx_environments_one_default_per_project
    ON environments (project_id)
    WHERE is_default = 1 AND scope = 'project';
