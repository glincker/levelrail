CREATE TABLE IF NOT EXISTS cache_purges (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    app        TEXT NOT NULL,
    domain     TEXT NOT NULL,
    scope      TEXT NOT NULL,
    value      TEXT NOT NULL DEFAULT '',
    removed    INTEGER NOT NULL DEFAULT 0,
    actor      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cache_purges_app ON cache_purges (app, id DESC);
