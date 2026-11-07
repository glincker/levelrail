-- Request headers of an http job as JSON: names and secret references only,
-- values are resolved from the app's env at run time.
ALTER TABLE scheduled_task_http ADD COLUMN headers TEXT NOT NULL DEFAULT '[]';
