-- Scheduled tasks: run an arbitrary command inside a running app's
-- container on a cron schedule (e.g. a nightly cleanup script, a
-- periodic cache-warm job). A child table of desired_services, the same
-- "one row per child resource, ON DELETE CASCADE" shape
-- service_domains (0012_service_domains.sql) already establishes,
-- rather than a JSON column on desired_services: an app can have any
-- number of scheduled tasks, each with its own independent schedule and
-- run history, so this needs real row identity, not a single nested
-- blob.
--
-- command is stored as a JSON array of strings (["sh", "-c", "..."]),
-- the same argv-array shape internal/api/exec.go's execRequest already
-- uses for a one-off exec, never shell-interpreted by this platform.
--
-- last_run_at/last_run_status/last_run_output are updated in place after
-- every run (scheduled or manual "run now"), not appended to a history
-- table: an operator needs "did this last run and how did it go", not a
-- full audit trail, matching this feature's own scope. last_run_output
-- is bounded to a fixed cap by the runner before it ever reaches this
-- column (internal/scheduledtask's own capped-output writer), never
-- unbounded command output.
CREATE TABLE scheduled_tasks (
    id               TEXT PRIMARY KEY,
    service_name     TEXT NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE,
    command          TEXT NOT NULL,
    schedule         TEXT NOT NULL,
    enabled          INTEGER NOT NULL DEFAULT 1,
    last_run_at      TEXT,
    last_run_status  TEXT NOT NULL DEFAULT '',
    last_run_output  TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

CREATE INDEX idx_scheduled_tasks_service_name ON scheduled_tasks(service_name);

-- Supports internal/scheduledtask.Scheduler.Tick's own
-- ListEnabledScheduledTasks query, run once per scheduler tick for the
-- lifetime of the process, the same "index the column your own frequent
-- query filters on" reasoning idx_desired_databases_backup_schedule
-- (0023_scheduled_backups.sql) already applies.
CREATE INDEX idx_scheduled_tasks_enabled ON scheduled_tasks(enabled) WHERE enabled = 1;
