-- One-time scheduled deploys: run_at is an alternative to cron, and
-- completed_at marks a one-shot that has been consumed. Both nullable so
-- existing recurring rows are unchanged.
ALTER TABLE app_schedules ADD COLUMN run_at TEXT;
ALTER TABLE app_schedules ADD COLUMN completed_at TEXT;
