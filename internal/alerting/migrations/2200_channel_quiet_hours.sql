CREATE TABLE channel_quiet_hours (
    channel_id TEXT PRIMARY KEY REFERENCES notification_channels(id) ON DELETE CASCADE,
    start_time TEXT NOT NULL,
    end_time   TEXT NOT NULL,
    timezone   TEXT NOT NULL DEFAULT 'UTC',
    enabled    INTEGER NOT NULL DEFAULT 1,
    updated_at TEXT NOT NULL
);
