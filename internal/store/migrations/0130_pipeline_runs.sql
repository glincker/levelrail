-- One row per pipeline run. definition is a snapshot of the YAML the run
-- started with so editing a pipeline never changes an in-flight run.
-- status: queued, running, succeeded, failed, cancelled. Every status
-- carries a reason string, shown in the UI.
CREATE TABLE pipeline_runs (
    id TEXT PRIMARY KEY,
    pipeline_id TEXT NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    app_name TEXT NOT NULL,
    number INTEGER NOT NULL,
    trigger_kind TEXT NOT NULL,
    trigger_actor TEXT NOT NULL DEFAULT '',
    ref TEXT NOT NULL DEFAULT '',
    commit_sha TEXT NOT NULL DEFAULT '',
    inputs TEXT NOT NULL DEFAULT '{}',
    definition TEXT NOT NULL,
    concurrency_group TEXT NOT NULL DEFAULT '',
    cancel_in_progress INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    cancel_requested INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    started_at TEXT,
    finished_at TEXT,
    UNIQUE (pipeline_id, number)
);

CREATE INDEX idx_pipeline_runs_pipeline ON pipeline_runs (pipeline_id, number DESC);
CREATE INDEX idx_pipeline_runs_app ON pipeline_runs (app_name, created_at DESC);
CREATE INDEX idx_pipeline_runs_status ON pipeline_runs (status);

-- One row per expanded job (a matrix job yields one row per combination).
-- status: pending, waiting_approval, running, succeeded, failed,
-- cancelled, skipped.
CREATE TABLE pipeline_jobs (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
    job_key TEXT NOT NULL,
    display_name TEXT NOT NULL,
    stage TEXT NOT NULL DEFAULT '',
    needs TEXT NOT NULL DEFAULT '[]',
    matrix TEXT NOT NULL DEFAULT '{}',
    node_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    attempt INTEGER NOT NULL DEFAULT 0,
    started_at TEXT,
    finished_at TEXT,
    UNIQUE (run_id, job_key)
);

CREATE INDEX idx_pipeline_jobs_run ON pipeline_jobs (run_id);

CREATE TABLE pipeline_steps (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL REFERENCES pipeline_jobs(id) ON DELETE CASCADE,
    idx INTEGER NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    exit_code INTEGER,
    attempt INTEGER NOT NULL DEFAULT 0,
    started_at TEXT,
    finished_at TEXT,
    UNIQUE (job_id, idx)
);
