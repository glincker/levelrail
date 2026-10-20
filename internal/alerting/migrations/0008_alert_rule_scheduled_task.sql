-- kind='scheduled_task_failure' rules watch one specific scheduled
-- task's consecutive-failure count (store.ScheduledTask.
-- ConsecutiveFailures), reusing restart_count_threshold as that
-- count's threshold (see internal/alerting/scheduled_task_failure.go).
-- resource_id stays the owning app's resource id, same as every other
-- app-scoped rule kind, so this column is only what picks out which of
-- that app's tasks the rule actually watches.
ALTER TABLE alert_rules ADD COLUMN scheduled_task_id TEXT NOT NULL DEFAULT '';
