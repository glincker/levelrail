-- Captured step output, one row per line. Capped per run in application
-- code; rows cascade away with their run.
CREATE TABLE pipeline_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
    job_key TEXT NOT NULL,
    step_idx INTEGER NOT NULL,
    stream TEXT NOT NULL DEFAULT 'stdout',
    line TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_pipeline_logs_run ON pipeline_logs (run_id, id);

-- Manual approval gates. decision is '' while pending, then 'approved'
-- or 'rejected'. required_ability is the API ability an approver needs.
CREATE TABLE pipeline_approvals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
    job_id TEXT NOT NULL REFERENCES pipeline_jobs(id) ON DELETE CASCADE,
    step_idx INTEGER NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    required_ability TEXT NOT NULL,
    decision TEXT NOT NULL DEFAULT '',
    decided_by TEXT NOT NULL DEFAULT '',
    comment TEXT NOT NULL DEFAULT '',
    expires_at TEXT,
    created_at TEXT NOT NULL,
    decided_at TEXT,
    UNIQUE (job_id, step_idx)
);

CREATE INDEX idx_pipeline_approvals_run ON pipeline_approvals (run_id);
