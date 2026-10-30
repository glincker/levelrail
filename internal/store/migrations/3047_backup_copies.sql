-- Second-destination copies of finished backups. One settings row, one copy row per backup.
CREATE TABLE backup_copy_settings (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	enabled INTEGER NOT NULL DEFAULT 0,
	target_id TEXT NOT NULL DEFAULT '',
	enabled_at TEXT NOT NULL DEFAULT ''
);
INSERT INTO backup_copy_settings (id) VALUES (1);

CREATE TABLE backup_copies (
	backup_history_id TEXT PRIMARY KEY REFERENCES backup_history(id) ON DELETE CASCADE,
	target_id TEXT NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('pending', 'copying', 'succeeded', 'failed')),
	attempts INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT '',
	size_bytes INTEGER NOT NULL DEFAULT 0,
	queued_at TEXT NOT NULL,
	copied_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_backup_copies_status ON backup_copies (status);
