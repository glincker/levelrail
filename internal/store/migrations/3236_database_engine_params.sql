-- Typed engine tuning parameters for managed databases. engine_params is what
-- the container runs with; a change is staged in engine_params_pending and only
-- moves over on an explicit apply or the next start, never on save.
ALTER TABLE desired_databases ADD COLUMN engine_params TEXT NOT NULL DEFAULT '{}';
ALTER TABLE desired_databases ADD COLUMN engine_params_pending TEXT;
ALTER TABLE desired_databases ADD COLUMN engine_params_previous TEXT NOT NULL DEFAULT '{}';
ALTER TABLE desired_databases ADD COLUMN engine_params_staged_by TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_databases ADD COLUMN engine_params_staged_at TEXT NOT NULL DEFAULT '';
