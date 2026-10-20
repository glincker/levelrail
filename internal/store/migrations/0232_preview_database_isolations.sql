-- Per-preview database isolation: an isolated Postgres role provisioned
-- on an existing managed database for one preview environment, tracked
-- here so teardown can find and drop it. database_name references an
-- existing desired_databases row, not one this table owns.
CREATE TABLE preview_database_isolations (
    id                      TEXT PRIMARY KEY,
    preview_environment_id  TEXT NOT NULL REFERENCES preview_environments (id),
    database_name           TEXT NOT NULL,
    source_key              TEXT NOT NULL,
    role_name               TEXT NOT NULL,
    secret_env_key          TEXT NOT NULL,
    status                  TEXT NOT NULL,
    status_reason           TEXT,
    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL
);

CREATE UNIQUE INDEX ux_preview_database_isolations_preview_source ON preview_database_isolations (preview_environment_id, source_key);
CREATE INDEX idx_preview_database_isolations_preview ON preview_database_isolations (preview_environment_id);
