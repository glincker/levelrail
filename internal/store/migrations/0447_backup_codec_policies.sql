-- Sealed volume backups: how an object was encoded (zstd, zstd+age) and a
-- content manifest taken from the raw archive while it streamed, so a restore
-- drill can prove restored files match what was backed up.
ALTER TABLE backup_history ADD COLUMN codec TEXT NOT NULL DEFAULT '';
ALTER TABLE backup_history ADD COLUMN manifest_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE backup_history ADD COLUMN manifest_files INTEGER NOT NULL DEFAULT 0;
ALTER TABLE backup_history ADD COLUMN manifest_bytes INTEGER NOT NULL DEFAULT 0;

-- Per-volume backup policy: grandfather-father-son retention counts and
-- consistency hooks. Separate from service_volume_backups so a volume can
-- carry a policy before (or without) a schedule.
CREATE TABLE volume_backup_policies (
    service_name   TEXT NOT NULL,
    volume_name    TEXT NOT NULL,
    retain_daily   INTEGER NOT NULL DEFAULT 0 CHECK (retain_daily >= 0),
    retain_weekly  INTEGER NOT NULL DEFAULT 0 CHECK (retain_weekly >= 0),
    retain_monthly INTEGER NOT NULL DEFAULT 0 CHECK (retain_monthly >= 0),
    pre_hook       TEXT NOT NULL DEFAULT '',
    post_hook      TEXT NOT NULL DEFAULT '',
    quiesce        TEXT NOT NULL DEFAULT '' CHECK (quiesce IN ('', 'pause')),
    updated_at     TEXT NOT NULL,
    PRIMARY KEY (service_name, volume_name)
);
