-- Automatic rollback record for engine tuning parameters. applied_at lets the
-- reconciler roll back only a set that was just applied, rolled_back_at stops a
-- second rollback until the next explicit apply.
ALTER TABLE desired_databases ADD COLUMN engine_params_applied_at TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_databases ADD COLUMN engine_params_rolled_back_at TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_databases ADD COLUMN engine_params_rolled_back_reason TEXT NOT NULL DEFAULT '';
