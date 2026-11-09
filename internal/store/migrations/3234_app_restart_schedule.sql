-- Scheduled restarts for apps, a cron or @every interval that recreates the
-- containers through the normal restart path. incarnation pins the row to one
-- life of the app name, so a recreated app never inherits it.
CREATE TABLE app_restart_schedule (
    app_name TEXT PRIMARY KEY,
    schedule TEXT NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    incarnation TEXT NOT NULL DEFAULT '',
    set_by TEXT NOT NULL DEFAULT '',
    set_at TEXT NOT NULL,
    next_run_at TEXT NOT NULL,
    last_run_at TEXT NOT NULL DEFAULT '',
    last_status TEXT NOT NULL DEFAULT '',
    last_reason TEXT NOT NULL DEFAULT '',
    failures INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX app_restart_schedule_next ON app_restart_schedule (next_run_at);
