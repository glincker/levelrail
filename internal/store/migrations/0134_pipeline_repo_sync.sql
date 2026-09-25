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

-- Run-level approval hold. A pull request from a fork can be held until an
-- approver releases it; hold_state is '' (not held), 'pending', 'approved',
-- or 'rejected'. No job starts, and no secret is resolved, while pending.
ALTER TABLE pipeline_runs ADD COLUMN hold_state TEXT NOT NULL DEFAULT '';
ALTER TABLE pipeline_runs ADD COLUMN hold_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE pipeline_runs ADD COLUMN hold_by TEXT NOT NULL DEFAULT '';
ALTER TABLE pipeline_runs ADD COLUMN hold_at TEXT;

-- Why a git event did or did not start a run. Pruned per app in
-- application code.
CREATE TABLE pipeline_trigger_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_name TEXT NOT NULL,
    pipeline_name TEXT NOT NULL DEFAULT '',
    event TEXT NOT NULL,
    ref TEXT NOT NULL DEFAULT '',
    sha TEXT NOT NULL DEFAULT '',
    decision TEXT NOT NULL,
    reason TEXT NOT NULL,
    run_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX idx_pipeline_trigger_log_app ON pipeline_trigger_log (app_name, id DESC);
