CREATE TABLE IF NOT EXISTS theauth_users (
    id                TEXT PRIMARY KEY,
    email             TEXT NOT NULL COLLATE NOCASE,
    email_verified_at INTEGER,
    name              TEXT NOT NULL DEFAULT '',
    avatar_url        TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    external_id       TEXT NOT NULL DEFAULT '',
    given_name        TEXT NOT NULL DEFAULT '',
    family_name       TEXT NOT NULL DEFAULT '',
    display_name      TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_users_email_uq ON theauth_users (email);

CREATE TABLE IF NOT EXISTS theauth_sessions (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES theauth_users (id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    ip         TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    revoked_at INTEGER,
    auth_level TEXT NOT NULL DEFAULT 'full'
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_sessions_token_hash_uq ON theauth_sessions (token_hash);
CREATE INDEX IF NOT EXISTS theauth_sessions_user_id_idx ON theauth_sessions (user_id);
CREATE INDEX IF NOT EXISTS theauth_sessions_expires_at_idx ON theauth_sessions (expires_at);

CREATE TABLE IF NOT EXISTS theauth_magic_links (
    id         TEXT PRIMARY KEY,
    email      TEXT NOT NULL COLLATE NOCASE,
    token_hash BLOB NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER,
    created_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_magic_links_token_hash_uq ON theauth_magic_links (token_hash);
CREATE INDEX IF NOT EXISTS theauth_magic_links_email_idx ON theauth_magic_links (email);
CREATE INDEX IF NOT EXISTS theauth_magic_links_expires_at_idx ON theauth_magic_links (expires_at);

CREATE TABLE IF NOT EXISTS theauth_user_passwords (
    user_id       TEXT PRIMARY KEY REFERENCES theauth_users (id) ON DELETE CASCADE,
    password_hash TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS theauth_password_reset_tokens (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES theauth_users (id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER,
    created_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_password_reset_tokens_hash_uq ON theauth_password_reset_tokens (token_hash);
CREATE INDEX IF NOT EXISTS theauth_password_reset_tokens_user_idx ON theauth_password_reset_tokens (user_id);
CREATE INDEX IF NOT EXISTS theauth_password_reset_tokens_expires_idx ON theauth_password_reset_tokens (expires_at);

CREATE TABLE IF NOT EXISTS theauth_oauth_accounts (
    id                TEXT PRIMARY KEY,
    user_id           TEXT NOT NULL REFERENCES theauth_users (id) ON DELETE CASCADE,
    provider          TEXT NOT NULL,
    provider_user_id  TEXT NOT NULL,
    access_token_enc  BLOB NOT NULL,
    refresh_token_enc BLOB,
    expires_at        INTEGER,
    scope             TEXT NOT NULL DEFAULT '',
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_oauth_accounts_provider_uq ON theauth_oauth_accounts (provider, provider_user_id);
CREATE INDEX IF NOT EXISTS theauth_oauth_accounts_user_idx ON theauth_oauth_accounts (user_id);

CREATE TABLE IF NOT EXISTS theauth_webauthn_credentials (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES theauth_users (id) ON DELETE CASCADE,
    credential_id   BLOB NOT NULL,
    public_key      BLOB NOT NULL,
    sign_count      INTEGER NOT NULL DEFAULT 0,
    transports      TEXT NOT NULL DEFAULT '[]',
    aaguid          BLOB NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    last_used_at    INTEGER,
    backup_eligible INTEGER,
    backup_state    INTEGER
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_webauthn_credential_id_uq ON theauth_webauthn_credentials (credential_id);
CREATE INDEX IF NOT EXISTS theauth_webauthn_user_idx ON theauth_webauthn_credentials (user_id);

CREATE TABLE IF NOT EXISTS theauth_totp_secrets (
    user_id      TEXT PRIMARY KEY REFERENCES theauth_users (id) ON DELETE CASCADE,
    secret_enc   BLOB NOT NULL,
    confirmed_at INTEGER,
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS theauth_totp_recovery_codes (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES theauth_users (id) ON DELETE CASCADE,
    code_hash  BLOB NOT NULL,
    used_at    INTEGER,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS theauth_totp_recovery_codes_user_idx ON theauth_totp_recovery_codes (user_id);

CREATE TABLE IF NOT EXISTS theauth_audit_events (
    id               TEXT PRIMARY KEY,
    organization_id  TEXT,
    actor_user_id    TEXT,
    actor_session_id TEXT,
    action           TEXT NOT NULL,
    target_type      TEXT NOT NULL DEFAULT '',
    target_id        TEXT NOT NULL DEFAULT '',
    metadata         TEXT NOT NULL DEFAULT '{}',
    ip               TEXT NOT NULL DEFAULT '',
    user_agent       TEXT NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS theauth_audit_org_created_idx ON theauth_audit_events (organization_id, created_at);
CREATE INDEX IF NOT EXISTS theauth_audit_actor_created_idx ON theauth_audit_events (actor_user_id, created_at);
CREATE INDEX IF NOT EXISTS theauth_audit_action_created_idx ON theauth_audit_events (action, created_at);

ALTER TABLE theauth_sessions ADD COLUMN last_seen_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE theauth_sessions ADD COLUMN elevated_until INTEGER;
ALTER TABLE theauth_sessions ADD COLUMN credential_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS theauth_sessions_credential_idx ON theauth_sessions (credential_id) WHERE credential_id <> '';

CREATE TABLE IF NOT EXISTS theauth_session_links (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES theauth_users (id) ON DELETE CASCADE,
    token_hash    BLOB NOT NULL,
    credential_id TEXT NOT NULL DEFAULT '',
    session_ttl   INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL,
    expires_at    INTEGER NOT NULL,
    consumed_at   INTEGER
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_session_links_token_hash_uq ON theauth_session_links (token_hash);
CREATE INDEX IF NOT EXISTS theauth_session_links_expires_idx ON theauth_session_links (expires_at);

CREATE TABLE IF NOT EXISTS theauth_api_tokens (
    id           TEXT PRIMARY KEY,
    owner_id     TEXT NOT NULL,
    owner_kind   TEXT NOT NULL,
    name         TEXT NOT NULL DEFAULT '',
    abilities    TEXT NOT NULL DEFAULT '[]',
    token_hash   BLOB NOT NULL,
    hint         TEXT NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL,
    expires_at   INTEGER,
    last_used_at INTEGER,
    revoked_at   INTEGER
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_api_tokens_hash_uq ON theauth_api_tokens (token_hash);
CREATE INDEX IF NOT EXISTS theauth_api_tokens_owner_idx ON theauth_api_tokens (owner_id);

CREATE TABLE IF NOT EXISTS theauth_device_codes (
    id                  TEXT PRIMARY KEY,
    device_code_hash    BLOB NOT NULL,
    user_code           TEXT NOT NULL,
    status              TEXT NOT NULL,
    client_name         TEXT NOT NULL DEFAULT '',
    requested_abilities TEXT NOT NULL DEFAULT '[]',
    approved_abilities  TEXT NOT NULL DEFAULT '[]',
    approver_id         TEXT,
    requester_ip        TEXT NOT NULL DEFAULT '',
    requester_ua        TEXT NOT NULL DEFAULT '',
    interval_seconds    INTEGER NOT NULL DEFAULT 5,
    last_polled_at      INTEGER,
    created_at          INTEGER NOT NULL,
    expires_at          INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_device_codes_hash_uq ON theauth_device_codes (device_code_hash);
CREATE UNIQUE INDEX IF NOT EXISTS theauth_device_codes_user_code_uq ON theauth_device_codes (user_code);
CREATE INDEX IF NOT EXISTS theauth_device_codes_expires_idx ON theauth_device_codes (expires_at);

CREATE TABLE IF NOT EXISTS theauth_totp_last_steps (
    user_id TEXT PRIMARY KEY REFERENCES theauth_users (id) ON DELETE CASCADE,
    step    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS theauth_throttle_entries (
    key           TEXT PRIMARY KEY,
    failures      INTEGER NOT NULL,
    last_failure  INTEGER NOT NULL,
    blocked_until INTEGER NOT NULL,
    expires_at    INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS theauth_throttle_entries_expires_idx ON theauth_throttle_entries (expires_at);

ALTER TABLE theauth_api_tokens ADD COLUMN kind TEXT NOT NULL DEFAULT '';
ALTER TABLE theauth_api_tokens ADD COLUMN agent_name TEXT NOT NULL DEFAULT '';
ALTER TABLE theauth_api_tokens ADD COLUMN delegated_by TEXT;

-- Side tables: library ids are ULIDs, platform ids are not, so each
-- migrated row keeps its legacy id here. Filled by the auth-backfill command.
CREATE TABLE IF NOT EXISTS authengine_user_map (
    legacy_id TEXT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    engine_id TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS authengine_token_map (
    legacy_id TEXT PRIMARY KEY REFERENCES api_tokens (id) ON DELETE CASCADE,
    engine_id TEXT NOT NULL UNIQUE
);
