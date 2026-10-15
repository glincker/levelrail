-- Object counts an engine reported while its backup was taken, recorded only
-- when they were stable across the snapshot, so a restore drill can compare.

CREATE TABLE backup_content_summaries (
    backup_history_id TEXT PRIMARY KEY REFERENCES backup_history(id) ON DELETE CASCADE,
    summary           TEXT NOT NULL,
    recorded_at       TEXT NOT NULL
);
