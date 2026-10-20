-- One rotation policy per app secret. Holds schedule and progress only,
-- never a value. previous_expires_at is set while the prior value is still
-- kept under <key>_PREVIOUS for the grace window.
CREATE TABLE secret_rotation_policies (
    service_name        TEXT NOT NULL,
    env_key             TEXT NOT NULL,
    interval_days       INTEGER NOT NULL,
    grace_hours         INTEGER NOT NULL DEFAULT 0,
    owner               TEXT NOT NULL DEFAULT '',
    auto_rotate         INTEGER NOT NULL DEFAULT 0,
    last_rotated_at     TEXT NOT NULL DEFAULT '',
    state               TEXT NOT NULL DEFAULT 'idle',
    last_error          TEXT NOT NULL DEFAULT '',
    previous_expires_at TEXT NOT NULL DEFAULT '',
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    PRIMARY KEY (service_name, env_key)
);
