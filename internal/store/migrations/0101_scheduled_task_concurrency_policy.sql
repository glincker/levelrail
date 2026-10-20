-- Governs whether a task's next due cron run may overlap a still-running
-- previous invocation (internal/scheduledtask.Runner): allow (default,
-- today's existing behavior) starts the new run unconditionally, forbid
-- skips it and records a distinct "skipped_concurrency" status, replace
-- cancels the in-flight run before starting the new one.
ALTER TABLE scheduled_tasks ADD COLUMN concurrency_policy TEXT NOT NULL DEFAULT 'allow';
