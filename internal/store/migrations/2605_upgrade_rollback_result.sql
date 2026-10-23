-- Self-upgrade attempts record whether a rollback restored the database and how many minutes of changes it discarded. No secrets.
ALTER TABLE self_upgrade_attempts ADD COLUMN rolled_back_minutes INTEGER NOT NULL DEFAULT -1;
ALTER TABLE self_upgrade_attempts ADD COLUMN database_restored INTEGER NOT NULL DEFAULT 0;
