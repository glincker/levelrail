-- A retired target keeps its row so backup history of deleted resources still
-- names it; every reader treats a non-empty deleted_at as gone.
ALTER TABLE backup_targets ADD COLUMN deleted_at TEXT NOT NULL DEFAULT '';
