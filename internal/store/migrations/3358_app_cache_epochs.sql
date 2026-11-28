-- Per app instance token that keys the build and registry cache, so a deleted
-- and recreated app of the same name never imports the previous owner's layers.
CREATE TABLE app_cache_epochs (
    name       TEXT PRIMARY KEY,
    epoch      TEXT NOT NULL,
    created_at TEXT NOT NULL
);
