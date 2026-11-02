-- Own cron schedule and target for physical base backups; empty schedule keeps
-- the base backup tied to each scheduled logical dump.
ALTER TABLE desired_databases ADD COLUMN base_backup_schedule TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_databases ADD COLUMN base_backup_target_id TEXT NOT NULL DEFAULT '';
