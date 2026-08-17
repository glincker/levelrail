-- App-level scheduled tasks: an arbitrary shell command, a cron
-- expression, and an enabled flag, run inside a service's currently
-- running container (internal/scheduledtask.Runner, reusing the same
-- Docker Engine API exec primitive POST /apps/{name}/exec already uses).
-- Command is always executed as `sh -c command`, matching an operator's
-- expectation of a cron line (pipes/redirection/env expansion work),
-- rather than exposing a separate argv-array shape.
--
-- ON DELETE CASCADE on service_name mirrors service_domains
-- (migrations/0012): a scheduled task's config has no meaning once its
-- owning service is gone.
CREATE TABLE scheduled_tasks (
    id              TEXT PRIMARY KEY,
    service_name    TEXT NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    command         TEXT NOT NULL,
    schedule        TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    timeout_seconds INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE INDEX idx_scheduled_tasks_service_name ON scheduled_tasks(service_name);

-- One row per attempted run, the scheduled-task counterpart of
-- backup_history (migrations/0018): a running row written before the
-- command starts, then updated to succeeded/failed once it finishes.
-- ON DELETE CASCADE on scheduled_task_id, unlike backup_history's own
-- no-cascade relationship to backup_targets: a run's history has no
-- independent meaning once the task that produced it is deleted.
CREATE TABLE scheduled_task_runs (
    id                TEXT PRIMARY KEY,
    scheduled_task_id TEXT NOT NULL REFERENCES scheduled_tasks(id) ON DELETE CASCADE,
    status            TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    exit_code         INTEGER NOT NULL DEFAULT 0,
    output            TEXT NOT NULL DEFAULT '',
    error             TEXT NOT NULL DEFAULT '',
    started_at        TEXT NOT NULL,
    finished_at       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_scheduled_task_runs_task_id ON scheduled_task_runs(scheduled_task_id, started_at DESC);
