-- One row per managed database upgrade attempt (scheduled or upgrade-now).
-- state is the resumable state machine; phase is the step inside it.
CREATE TABLE db_upgrade_runs (
    id                TEXT PRIMARY KEY,
    database_name     TEXT NOT NULL,
    engine            TEXT NOT NULL,
    from_version      TEXT NOT NULL,
    to_version        TEXT NOT NULL,
    kind              TEXT NOT NULL CHECK (kind IN ('patch', 'minor', 'major')),
    source            TEXT NOT NULL CHECK (source IN ('auto', 'manual')),
    state             TEXT NOT NULL CHECK (state IN ('pending', 'backing_up', 'upgrading', 'verifying', 'succeeded', 'reverted', 'failed')),
    phase             TEXT NOT NULL DEFAULT '',
    verify_after      INTEGER NOT NULL DEFAULT 1,
    revert_on_failure INTEGER NOT NULL DEFAULT 1,
    notify_json       TEXT NOT NULL DEFAULT '[]',
    backup_id         TEXT NOT NULL DEFAULT '',
    verification_id   TEXT NOT NULL DEFAULT '',
    from_image_digest TEXT NOT NULL DEFAULT '',
    snapshot_volume   TEXT NOT NULL DEFAULT '',
    revert_path       TEXT NOT NULL DEFAULT '',
    reason            TEXT NOT NULL DEFAULT '',
    requested_by      TEXT NOT NULL DEFAULT '',
    timings_json      TEXT NOT NULL DEFAULT '{}',
    created_at        TEXT NOT NULL,
    phase_started_at  TEXT NOT NULL DEFAULT '',
    finished_at       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_db_upgrade_runs_db ON db_upgrade_runs (database_name, created_at DESC);
CREATE INDEX idx_db_upgrade_runs_state ON db_upgrade_runs (state);
