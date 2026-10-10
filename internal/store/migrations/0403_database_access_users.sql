CREATE TABLE IF NOT EXISTS database_access_users (
    id            TEXT PRIMARY KEY,
    database_name TEXT NOT NULL,
    role          TEXT NOT NULL,
    kind          TEXT NOT NULL,
    preset        TEXT NOT NULL,
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    expires_at    TEXT,
    state         TEXT NOT NULL DEFAULT 'active',
    claimed_at    TEXT,
    revoked_at    TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_database_access_users_live
    ON database_access_users (database_name, role) WHERE state != 'revoked';
CREATE INDEX IF NOT EXISTS idx_database_access_users_due
    ON database_access_users (kind, state, expires_at);
