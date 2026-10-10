-- Browsers a user has approved for password sign-in. token_hash is the
-- SHA-256 of the trusted-device cookie; the cookie value is never stored.
CREATE TABLE IF NOT EXISTS trusted_devices (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    label        TEXT NOT NULL DEFAULT '',
    ip           TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    last_used_at TEXT NOT NULL,
    expires_at   TEXT NOT NULL,
    revoked_at   TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_trusted_devices_user ON trusted_devices (user_id);

-- Single row: whether sign in with a code is offered to admin and to other
-- accounts. No row means the APP_AUTH_CODE_LOGIN_* defaults apply.
CREATE TABLE IF NOT EXISTS auth_code_login_settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    admins     INTEGER NOT NULL,
    others     INTEGER NOT NULL,
    updated_at TEXT NOT NULL
);
