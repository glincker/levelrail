-- Per-app scheduled redeploys: a cron expression plus a branch, checked
-- by internal/scheduledeploy.Scheduler. next_fire_at is persisted (not
-- kept only in memory) so a control plane restart can still tell whether
-- a schedule's due time has passed, see that package's own doc comment
-- for the catch-up-at-most-once semantics this enables.
CREATE TABLE IF NOT EXISTS app_schedules (
    service_name   TEXT PRIMARY KEY,
    cron           TEXT NOT NULL,
    branch         TEXT NOT NULL,
    timezone       TEXT NOT NULL DEFAULT 'UTC',
    enabled        INTEGER NOT NULL DEFAULT 1,
    next_fire_at   TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS app_schedule_history (
    id             TEXT PRIMARY KEY,
    service_name   TEXT NOT NULL,
    scheduled_for  TEXT NOT NULL,
    fired_at       TEXT NOT NULL,
    status         TEXT NOT NULL,
    reason         TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_app_schedule_history_service_time ON app_schedule_history (service_name, fired_at DESC, id DESC);
