-- Pre-upgrade restore points (database copy, binary, unit file) kept on the host; file paths derive from the id. No secrets.
CREATE TABLE upgrade_restore_points (
    id             TEXT PRIMARY KEY,
    version        TEXT NOT NULL,
    schema_version INTEGER NOT NULL DEFAULT -1,
    attempt_id     TEXT NOT NULL DEFAULT '',
    size_bytes     INTEGER NOT NULL DEFAULT 0,
    has_binary     INTEGER NOT NULL DEFAULT 0,
    has_unit       INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL
);
CREATE INDEX upgrade_restore_points_created ON upgrade_restore_points (created_at DESC);
