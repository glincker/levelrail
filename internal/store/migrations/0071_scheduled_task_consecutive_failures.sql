-- Tracks how many runs in a row a scheduled task has failed
-- (RecordScheduledTaskRun resets this to 0 on success, increments it
-- otherwise), so a kind=scheduled_task_failure alert rule
-- (internal/alerting) can fire on repeated failure without re-deriving
-- a run history this table never kept.
ALTER TABLE scheduled_tasks ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0;
