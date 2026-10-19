-- Container teardowns still owed after a database was deleted or moved; retried until the runtime succeeds.
CREATE TABLE database_teardown_jobs (
    name        TEXT NOT NULL,
    node_id     TEXT NOT NULL DEFAULT '',
    purge       INTEGER NOT NULL DEFAULT 0,
    attempts    INTEGER NOT NULL DEFAULT 0,
    last_error  TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    PRIMARY KEY (name, node_id)
);
