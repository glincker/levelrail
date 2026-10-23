-- Step-level progress of creating a database. One row per step transition;
-- an attempt is every row sharing attempt_id.
CREATE TABLE database_provision_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    database_name TEXT NOT NULL,
    attempt_id    TEXT NOT NULL,
    step          TEXT NOT NULL,
    status        TEXT NOT NULL,
    detail        TEXT NOT NULL DEFAULT '',
    current       INTEGER NOT NULL DEFAULT 0,
    total         INTEGER NOT NULL DEFAULT 0,
    at            TEXT NOT NULL
);

CREATE INDEX database_provision_events_db ON database_provision_events (database_name, id);
CREATE INDEX database_provision_events_attempt ON database_provision_events (attempt_id);
