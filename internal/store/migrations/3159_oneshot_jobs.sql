-- One-shot job summaries, so a status poll survives a control plane restart.
-- Per-attempt runs stay in scheduled_task_runs; this holds the job rollup.
CREATE TABLE IF NOT EXISTS oneshot_jobs (
    id              TEXT PRIMARY KEY,
    app             TEXT NOT NULL,
    task_id         TEXT NOT NULL DEFAULT '',
    state           TEXT NOT NULL,
    completions     INTEGER NOT NULL DEFAULT 1,
    parallelism     INTEGER NOT NULL DEFAULT 1,
    backoff_limit   INTEGER NOT NULL DEFAULT 0,
    succeeded       INTEGER NOT NULL DEFAULT 0,
    failed_attempts INTEGER NOT NULL DEFAULT 0,
    active          INTEGER NOT NULL DEFAULT 0,
    run_ids         TEXT NOT NULL DEFAULT '[]',
    message         TEXT NOT NULL DEFAULT '',
    started_at      TEXT NOT NULL DEFAULT '',
    finished_at     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_oneshot_jobs_started ON oneshot_jobs (started_at);
