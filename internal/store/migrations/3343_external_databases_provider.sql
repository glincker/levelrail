-- Hosting service a connected database runs on (neon, aws-rds, generic, ...).
-- Empty for databases connected without the hosted flow.
ALTER TABLE external_databases ADD COLUMN provider TEXT NOT NULL DEFAULT '';
