-- One row per host-run self-upgrade attempt. The host command journals each
-- attempt to a file and the control plane imports it at boot, so a rolled-back
-- attempt (database restored from its snapshot) is still recorded.
CREATE TABLE self_upgrade_attempts (
    id               TEXT PRIMARY KEY,
    from_version     TEXT NOT NULL,
    to_version       TEXT NOT NULL,
    from_schema      INTEGER NOT NULL DEFAULT -1,
    to_schema        INTEGER NOT NULL DEFAULT -1,
    initiator        TEXT NOT NULL DEFAULT '',
    outcome          TEXT NOT NULL CHECK (outcome IN ('running', 'succeeded', 'rolled_back', 'refused', 'failed')),
    failed_step      TEXT NOT NULL DEFAULT '',
    error            TEXT NOT NULL DEFAULT '',
    backup_name      TEXT NOT NULL DEFAULT '',
    acked_json       TEXT NOT NULL DEFAULT '[]',
    steps_json       TEXT NOT NULL DEFAULT '[]',
    started_at       TEXT NOT NULL,
    finished_at      TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_self_upgrade_attempts_started ON self_upgrade_attempts (started_at DESC);
