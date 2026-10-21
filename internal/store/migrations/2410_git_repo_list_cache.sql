-- Per-connection cache of provider repository lists, so the repo picker does not re-list a whole account on every open.
CREATE TABLE git_repo_list_cache (
    cache_key  TEXT PRIMARY KEY,
    provider   TEXT NOT NULL,
    payload    TEXT NOT NULL,
    meta       TEXT NOT NULL DEFAULT '',
    truncated  INTEGER NOT NULL DEFAULT 0,
    fetched_at TEXT NOT NULL
);
