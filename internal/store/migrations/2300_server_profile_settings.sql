-- Operator's server profile (minimal, standard or suite). Absent row means
-- never chosen; APP_SERVER_PROFILE wins over this row when set.
CREATE TABLE server_profile_settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    profile    TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL DEFAULT ''
);
