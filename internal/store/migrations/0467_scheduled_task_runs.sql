-- One row per job run (scheduled, manual, or api), so history survives
-- beyond the single last_run_* snapshot on scheduled_tasks. The partial
-- unique index on (task_id, scheduled_for) is the idempotency claim: a
-- scheduler that restarts and re-derives the same cron occurrence cannot
-- insert a second run for it.
CREATE TABLE scheduled_task_runs (
    id            TEXT PRIMARY KEY,
    task_id       TEXT NOT NULL REFERENCES scheduled_tasks(id) ON DELETE CASCADE,
    service_name  TEXT NOT NULL,
    trigger_kind  TEXT NOT NULL,
    status        TEXT NOT NULL,
    scheduled_for TEXT,
    started_at    TEXT NOT NULL,
    finished_at   TEXT,
    exit_code     INTEGER,
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    attempts      INTEGER NOT NULL DEFAULT 0,
    node_id       TEXT NOT NULL DEFAULT '',
    mode          TEXT NOT NULL DEFAULT '',
    container_id  TEXT NOT NULL DEFAULT '',
    message       TEXT NOT NULL DEFAULT '',
    output        TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_scheduled_task_runs_task_started ON scheduled_task_runs(task_id, started_at DESC);
CREATE INDEX idx_scheduled_task_runs_service_started ON scheduled_task_runs(service_name, started_at DESC);
CREATE INDEX idx_scheduled_task_runs_status ON scheduled_task_runs(status) WHERE status = 'running';
CREATE UNIQUE INDEX idx_scheduled_task_runs_claim
    ON scheduled_task_runs(task_id, scheduled_for) WHERE scheduled_for IS NOT NULL;
