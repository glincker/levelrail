-- Per-app ignore_paths (probes that never count as traffic), port wake on
-- or off, and the last failed wake so a failure stays visible.
ALTER TABLE app_sleep ADD COLUMN ignore_paths TEXT NOT NULL DEFAULT '[]';
ALTER TABLE app_sleep ADD COLUMN port_wake INTEGER NOT NULL DEFAULT 1;
ALTER TABLE app_sleep ADD COLUMN wake_error TEXT NOT NULL DEFAULT '';
