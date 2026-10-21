-- HTTP scheduled invocations: a scheduled_tasks row with mode 'http' calls a
-- path on its app instead of running a command. Kept beside the task, not on
-- it, so the wide scheduled_tasks column list stays untouched. For http runs
-- scheduled_task_runs.exit_code carries the response status code.
CREATE TABLE scheduled_task_http (
    task_id    TEXT PRIMARY KEY REFERENCES scheduled_tasks(id) ON DELETE CASCADE,
    method     TEXT NOT NULL DEFAULT 'POST',
    path       TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    background INTEGER NOT NULL DEFAULT 0
);
