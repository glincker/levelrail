-- One row per scheduled control plane upgrade. At most one is active
-- (pending, blocked or running) at a time; the scheduler transitions a row
-- with compare-and-set so a restart mid-fire never applies twice.
CREATE TABLE upgrade_schedules (
    id             TEXT PRIMARY KEY,
    target_version TEXT NOT NULL,
    source         TEXT NOT NULL CHECK (source IN ('manual', 'auto')),
    mode           TEXT NOT NULL CHECK (mode IN ('at', 'window')),
    run_at         TEXT NOT NULL,
    retry_until    TEXT NOT NULL,
    timezone       TEXT NOT NULL DEFAULT 'UTC',
    state          TEXT NOT NULL CHECK (state IN ('pending', 'blocked', 'running', 'applied', 'failed', 'cancelled', 'missed')),
    acked_json     TEXT NOT NULL DEFAULT '[]',
    major_acked    INTEGER NOT NULL DEFAULT 0,
    created_by     TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    check_count    INTEGER NOT NULL DEFAULT 0,
    last_check_at  TEXT NOT NULL DEFAULT '',
    last_reason    TEXT NOT NULL DEFAULT '',
    initiator      TEXT NOT NULL DEFAULT '',
    fired_at       TEXT NOT NULL DEFAULT '',
    finished_at    TEXT NOT NULL DEFAULT '',
    notified_state TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_upgrade_schedules_state ON upgrade_schedules (state, run_at);
CREATE INDEX idx_upgrade_schedules_created ON upgrade_schedules (created_at DESC);
