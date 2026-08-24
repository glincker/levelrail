-- backup_retain_days: second, independent retention dimension alongside
-- backup_retain (migrations/0023_scheduled_backups.sql). 0 means "no
-- age limit", the same convention backup_retain already uses for count.
ALTER TABLE desired_databases ADD COLUMN backup_retain_days INTEGER NOT NULL DEFAULT 0;
