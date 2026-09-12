-- One row per (preview environment, app.yaml database key) that opted
-- into ephemeralInPreviews: the disposable database.Controller-managed
-- instance provisioned alongside a preview and destroyed with it (see
-- internal/api/preview_environments_databases.go). database_name is the
-- desired_databases.name this row's own container reconciles against;
-- deleted once teardown fully succeeds, kept with status
-- 'teardown_failed' and a reason when it doesn't, the same
-- keep-the-row-for-retry convention preview_environments itself already
-- uses for a partially failed teardown.
CREATE TABLE preview_ephemeral_databases (
    id                      TEXT PRIMARY KEY,
    preview_environment_id  TEXT NOT NULL REFERENCES preview_environments (id),
    database_name           TEXT NOT NULL UNIQUE,
    source_key              TEXT NOT NULL,
    engine                  TEXT NOT NULL,
    version                 TEXT NOT NULL,
    status                  TEXT NOT NULL,
    status_reason           TEXT,
    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL
);

CREATE UNIQUE INDEX ux_preview_ephemeral_databases_preview_source ON preview_ephemeral_databases (preview_environment_id, source_key);
CREATE INDEX idx_preview_ephemeral_databases_preview ON preview_ephemeral_databases (preview_environment_id);
