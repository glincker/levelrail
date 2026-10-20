-- Last known health of each git provider connection, so a revoked grant shows a Reconnect prompt instead of a generic error.
CREATE TABLE git_connection_status (
    provider        TEXT PRIMARY KEY,
    status          TEXT NOT NULL,
    detail          TEXT NOT NULL DEFAULT '',
    next_check_at   TEXT NOT NULL DEFAULT '',
    updated_at      TEXT NOT NULL
);
