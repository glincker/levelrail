-- Undo window for deleted apps and databases: the config snapshot and a
-- reference to the last backup stay until the purge job drops the row.
CREATE TABLE recently_deleted (
    kind        TEXT NOT NULL CHECK (kind IN ('app', 'database')),
    name        TEXT NOT NULL,
    snapshot    TEXT NOT NULL,
    last_backup TEXT NOT NULL DEFAULT '',
    deleted_by  TEXT NOT NULL DEFAULT '',
    deleted_at  TEXT NOT NULL,
    PRIMARY KEY (kind, name)
);
CREATE INDEX recently_deleted_deleted_at ON recently_deleted (deleted_at);
