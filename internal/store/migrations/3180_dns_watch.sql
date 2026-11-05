-- Domains the DNS watcher follows until live, so a restart resumes them and a live notice is sent once.
CREATE TABLE dns_watch (
    domain       TEXT PRIMARY KEY,
    app          TEXT NOT NULL DEFAULT '',
    state        TEXT NOT NULL DEFAULT 'waiting_dns',
    since        TEXT NOT NULL,
    last_checked TEXT NOT NULL DEFAULT '',
    next_check   TEXT NOT NULL DEFAULT '',
    attempts     INTEGER NOT NULL DEFAULT 0,
    stalled_at   TEXT NOT NULL DEFAULT '',
    stopped      INTEGER NOT NULL DEFAULT 0,
    notified_at  TEXT NOT NULL DEFAULT ''
);
