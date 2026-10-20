-- Automatic upgrade policy per managed database. The row named '*' is the
-- platform default; a database without its own row inherits it.
CREATE TABLE db_upgrade_policies (
    database_name           TEXT PRIMARY KEY,
    auto_upgrade            TEXT NOT NULL DEFAULT 'off' CHECK (auto_upgrade IN ('off', 'patch', 'minor')),
    window_cron             TEXT NOT NULL DEFAULT '',
    window_duration_seconds INTEGER NOT NULL DEFAULT 0 CHECK (window_duration_seconds >= 0),
    window_timezone         TEXT NOT NULL DEFAULT 'UTC',
    backup_before           INTEGER NOT NULL DEFAULT 1 CHECK (backup_before = 1),
    verify_after            INTEGER NOT NULL DEFAULT 1,
    revert_on_failure       INTEGER NOT NULL DEFAULT 1,
    notify_json             TEXT NOT NULL DEFAULT '[]',
    updated_by              TEXT NOT NULL DEFAULT '',
    updated_at              TEXT NOT NULL
);

INSERT INTO db_upgrade_policies (database_name, window_cron, window_duration_seconds, updated_at)
VALUES ('*', '0 3 * * 0', 7200, '1970-01-01T00:00:00Z');
