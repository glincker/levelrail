CREATE TABLE database_password_rotation (
    database_name     TEXT PRIMARY KEY,
    interval_days     INTEGER NOT NULL DEFAULT 0,
    state             TEXT NOT NULL DEFAULT 'idle',
    last_rotated_at   TEXT NOT NULL DEFAULT '',
    last_error        TEXT NOT NULL DEFAULT '',
    pending_since     TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);
