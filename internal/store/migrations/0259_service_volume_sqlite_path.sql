-- Non-empty: path within the volume to a .db file the runner backs up
-- via sqlite3 .backup instead of a raw tar (safe under concurrent
-- writes; a filesystem copy of a live SQLite file isn't).
ALTER TABLE service_volume_backups ADD COLUMN sqlite_path TEXT NOT NULL DEFAULT '';
