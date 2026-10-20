-- One git push deploy remote per app. Only the SHA-256 of the push token is stored.
CREATE TABLE git_push_remotes (
    service_name     TEXT PRIMARY KEY REFERENCES desired_services(name) ON DELETE CASCADE,
    token_hash       TEXT NOT NULL,
    branch           TEXT NOT NULL DEFAULT 'main',
    created_at       TEXT NOT NULL,
    rotated_at       TEXT NOT NULL DEFAULT '',
    last_push_at     TEXT NOT NULL DEFAULT '',
    last_push_commit TEXT NOT NULL DEFAULT '',
    last_push_result TEXT NOT NULL DEFAULT ''
);
