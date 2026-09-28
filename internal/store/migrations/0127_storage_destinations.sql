-- Per-destination options layered on backup_targets so a target doubles as a
-- general storage destination without rebuilding that table.
CREATE TABLE storage_destination_options (
    target_id  TEXT PRIMARY KEY REFERENCES backup_targets(id) ON DELETE CASCADE,
    preset     TEXT NOT NULL DEFAULT 'custom',
    path_style INTEGER NOT NULL DEFAULT 1,
    account_id TEXT NOT NULL DEFAULT ''
);

-- One row per log archive policy. app_name '' is the global policy.
-- watermark_ns is the exclusive upper bound (unix nanos) already archived.
CREATE TABLE log_archive_policies (
    id               TEXT PRIMARY KEY,
    app_name         TEXT NOT NULL DEFAULT '',
    target_id        TEXT NOT NULL REFERENCES backup_targets(id),
    enabled          INTEGER NOT NULL DEFAULT 1,
    interval_seconds INTEGER NOT NULL,
    retention_days   INTEGER NOT NULL DEFAULT 0,
    watermark_ns     INTEGER NOT NULL,
    last_run_at      TEXT NOT NULL DEFAULT '',
    last_success_at  TEXT NOT NULL DEFAULT '',
    last_error       TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL,
    UNIQUE (app_name)
);
