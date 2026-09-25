-- Repository sync for pipeline definitions. source_sha is the commit a
-- 'repo' pipeline was last synced from; synced_hash is the SHA-256 of the
-- YAML as synced, so an edit made afterwards shows as diverged.
ALTER TABLE pipelines ADD COLUMN source_sha TEXT NOT NULL DEFAULT '';
ALTER TABLE pipelines ADD COLUMN synced_hash TEXT NOT NULL DEFAULT '';

-- Per-app sync settings and the outcome of the last sync attempt.
-- repo_is_truth lets a sync overwrite definitions edited in the dashboard.
CREATE TABLE pipeline_sync (
    app_name TEXT PRIMARY KEY,
    repo_is_truth INTEGER NOT NULL DEFAULT 0,
    last_sha TEXT NOT NULL DEFAULT '',
    last_synced_at TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT ''
);
