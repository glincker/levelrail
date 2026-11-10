-- Job guard rails: failure alert opt-in and per-job run retention. They live
-- on the job row, so a deleted job takes them with it and a recreated job
-- (new id) starts clean.
ALTER TABLE scheduled_tasks ADD COLUMN alert_on_failure INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduled_tasks ADD COLUMN keep_runs INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduled_tasks ADD COLUMN keep_days INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_scheduled_task_runs_task_status ON scheduled_task_runs(task_id, status);

-- Failure alert rate limit marks, one per alert rule and job. Times are fixed
-- width UTC text; alerted_run_at is the job's last_run_at the alert covered.
CREATE TABLE job_failure_alerts (
    rule_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    last_alert_at TEXT NOT NULL,
    alerted_run_at TEXT NOT NULL,
    prev_alert_at TEXT,
    prev_run_at TEXT,
    PRIMARY KEY (rule_id, task_id)
);

-- Queued runs and alert marks must never outlive their job.
CREATE TRIGGER scheduled_tasks_drop_guard_rows AFTER DELETE ON scheduled_tasks
BEGIN
    DELETE FROM scheduled_task_runs WHERE task_id = OLD.id AND status = 'queued';
    DELETE FROM job_failure_alerts WHERE task_id = OLD.id;
END;
