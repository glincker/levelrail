-- Memory/CPU caps for a managed database, the same JSON-column shape
-- desired_services.resources already uses (0002_desired_services.sql):
-- read and written as one whole nested structure, not queried by
-- sub-field.
ALTER TABLE desired_databases ADD COLUMN resources TEXT NOT NULL DEFAULT '{}';
