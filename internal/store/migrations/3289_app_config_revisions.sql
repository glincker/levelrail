-- Versioned app definition history: one row per recorded change to env,
-- scale, resources, domains or health. Secret values are never stored here.
CREATE TABLE app_config_revisions (
    app_name  TEXT NOT NULL,
    n         INTEGER NOT NULL,
    at        TEXT NOT NULL,
    actor     TEXT NOT NULL DEFAULT '',
    revert_of INTEGER NOT NULL DEFAULT 0,
    snapshot  TEXT NOT NULL,
    changes   TEXT NOT NULL DEFAULT '[]',
    PRIMARY KEY (app_name, n)
);
