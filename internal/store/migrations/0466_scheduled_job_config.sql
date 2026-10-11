-- Scheduled jobs: richer per-task configuration on top of the original
-- exec-only scheduled_tasks row. mode is exec (run inside the running
-- container, the pre-0466 behavior, so existing rows keep it) or fresh
-- (a new one-off container from the app's image with its env, volumes and
-- network). last_scheduled_for is the level-triggered high-water mark of
-- the cron occurrence last handled, armed_at is when the schedule last
-- became live, so a restart neither double-runs nor backfills a job that
-- was just enabled.
ALTER TABLE scheduled_tasks ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE scheduled_tasks ADD COLUMN mode TEXT NOT NULL DEFAULT 'exec';
ALTER TABLE scheduled_tasks ADD COLUMN timezone TEXT NOT NULL DEFAULT 'UTC';
ALTER TABLE scheduled_tasks ADD COLUMN timeout_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduled_tasks ADD COLUMN max_retries INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduled_tasks ADD COLUMN retry_backoff_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduled_tasks ADD COLUMN catch_up TEXT NOT NULL DEFAULT 'skip';
ALTER TABLE scheduled_tasks ADD COLUMN catch_up_lookback_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduled_tasks ADD COLUMN cpu_limit_millis INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduled_tasks ADD COLUMN memory_limit_mb INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduled_tasks ADD COLUMN warn_after_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduled_tasks ADD COLUMN source TEXT NOT NULL DEFAULT 'api';
ALTER TABLE scheduled_tasks ADD COLUMN armed_at TEXT;
ALTER TABLE scheduled_tasks ADD COLUMN last_scheduled_for TEXT;
ALTER TABLE scheduled_tasks ADD COLUMN running_since TEXT;
ALTER TABLE scheduled_tasks ADD COLUMN consecutive_missed INTEGER NOT NULL DEFAULT 0;

UPDATE scheduled_tasks SET armed_at = created_at WHERE armed_at IS NULL;

CREATE UNIQUE INDEX idx_scheduled_tasks_service_name_unique
    ON scheduled_tasks(service_name, name) WHERE name != '';
