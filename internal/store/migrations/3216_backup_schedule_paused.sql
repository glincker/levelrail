-- A paused schedule keeps its cron, target and retention; only the scheduler skips it.
ALTER TABLE desired_databases ADD COLUMN backup_paused INTEGER NOT NULL DEFAULT 0;
ALTER TABLE service_volume_backups ADD COLUMN backup_paused INTEGER NOT NULL DEFAULT 0;
