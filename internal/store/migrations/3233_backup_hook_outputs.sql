-- Masked output of a failing volume backup hook, kept apart from the history
-- row's error so only callers allowed to change hooks can read it.
CREATE TABLE backup_hook_outputs (
	backup_history_id TEXT PRIMARY KEY REFERENCES backup_history(id) ON DELETE CASCADE,
	phase             TEXT NOT NULL,
	output            TEXT NOT NULL,
	created_at        TEXT NOT NULL
);
