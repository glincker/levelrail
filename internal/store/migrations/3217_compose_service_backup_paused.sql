-- A paused compose database schedule keeps its cron and target; only the scheduler skips it.
ALTER TABLE compose_service_backups ADD COLUMN backup_paused INTEGER NOT NULL DEFAULT 0;
