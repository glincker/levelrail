-- Platform security policy (single row). A NULL column means the APP_*
-- env default applies; no row at all means every default applies.
CREATE TABLE IF NOT EXISTS security_policy_settings (
    id                      INTEGER PRIMARY KEY CHECK (id = 1),
    approval_scope          TEXT,
    max_token_lifetime_days INTEGER,
    warn_unused_days        INTEGER,
    disable_unused_days     INTEGER,
    updated_at              TEXT NOT NULL
);

-- Per-account security switches and the "this wasn't me" reset flag.
CREATE TABLE IF NOT EXISTS user_security_settings (
    user_id                     TEXT PRIMARY KEY,
    require_new_device_approval INTEGER NOT NULL DEFAULT 0,
    reset_flagged_at            TEXT NOT NULL DEFAULT '',
    reset_flag_reason           TEXT NOT NULL DEFAULT '',
    updated_at                  TEXT NOT NULL
);

-- One row per new-browser sign-in alert link. The link is an HMAC over the
-- id; this row makes it single use and names what it revokes.
CREATE TABLE IF NOT EXISTS sign_in_alert_tokens (
    id                TEXT PRIMARY KEY,
    user_id           TEXT NOT NULL,
    session_id        TEXT NOT NULL DEFAULT '',
    trusted_device_id TEXT NOT NULL DEFAULT '',
    ip                TEXT NOT NULL DEFAULT '',
    label             TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL,
    expires_at        TEXT NOT NULL,
    used_at           TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_sign_in_alert_tokens_expiry ON sign_in_alert_tokens (expires_at);

-- Unused-token sweep: a notice first, a revoke only after the grace period.
CREATE TABLE IF NOT EXISTS token_hygiene_notices (
    token_id    TEXT PRIMARY KEY,
    owner_id    TEXT NOT NULL DEFAULT '',
    token_name  TEXT NOT NULL DEFAULT '',
    noticed_at  TEXT NOT NULL,
    disabled_at TEXT NOT NULL DEFAULT ''
);
