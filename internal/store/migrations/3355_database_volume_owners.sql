-- Team and project that own a database data volume, kept after the database
-- is deleted so a same-name database of another team cannot reuse the data.
CREATE TABLE database_volume_owners (
    name        TEXT PRIMARY KEY,
    team_id     TEXT NOT NULL,
    project_id  TEXT NOT NULL DEFAULT '',
    recorded_at TEXT NOT NULL
);
