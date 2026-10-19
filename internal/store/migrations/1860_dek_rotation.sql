ALTER TABLE service_secrets ADD COLUMN rotated_at TEXT NOT NULL DEFAULT '';

CREATE TABLE secret_key_policy (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    max_key_age_days    INTEGER NOT NULL DEFAULT 0,
    remind_every_days   INTEGER NOT NULL DEFAULT 7,
    last_reminded_at    TEXT NOT NULL DEFAULT '',
    updated_at          TEXT NOT NULL,
    updated_by          TEXT NOT NULL DEFAULT ''
);

CREATE TABLE dek_rotation_runs (
    id          TEXT PRIMARY KEY,
    started_at  TEXT NOT NULL,
    finished_at TEXT NOT NULL DEFAULT '',
    total       INTEGER NOT NULL DEFAULT 0,
    done        INTEGER NOT NULL DEFAULT 0,
    failed      INTEGER NOT NULL DEFAULT 0,
    actor       TEXT NOT NULL DEFAULT ''
);
