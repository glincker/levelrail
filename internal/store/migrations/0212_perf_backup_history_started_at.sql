-- Missing index for pagination queries scanning backup_history
CREATE INDEX idx_backup_history_started_at ON backup_history(started_at DESC);
