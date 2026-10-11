-- Every compose file a stack was deployed from, so a rollback can re-apply
-- the previous one. Generated secrets live in the secret store keyed by app,
-- so re-applying an old file reuses the same values.
CREATE TABLE compose_revisions (
    app_id      TEXT NOT NULL,
    revision    INTEGER NOT NULL CHECK (revision > 0),
    compose     TEXT NOT NULL,
    profiles    TEXT NOT NULL DEFAULT '[]',
    source      TEXT NOT NULL DEFAULT 'api',
    template_id TEXT NOT NULL DEFAULT '',
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    PRIMARY KEY (app_id, revision)
);
