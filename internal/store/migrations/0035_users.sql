-- Real multi-user identity, replacing admin_user's Phase 1 singleton
-- (0005_admin_user.sql). No role column: RBAC stays out of scope until
-- Phase 4. is_first_user is display-only, never used for access control.
CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    email         TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    password_hash TEXT,
    is_first_user INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL,
    last_login_at TEXT
);

CREATE UNIQUE INDEX ux_users_email ON users (email);

-- Partial unique index: SQLite has no direct "at most one row where X is
-- true" constraint, so this is how "at most one first user, ever" gets
-- enforced at the database level.
CREATE UNIQUE INDEX ux_users_single_first_user ON users (is_first_user) WHERE is_first_user = 1;

-- ux_oauth_identities_provider_account below is the account-takeover
-- guard: it makes linking an already-linked external account to a
-- second user a constraint violation, not an application-level check.
CREATE TABLE user_oauth_identities (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider         TEXT NOT NULL CHECK (provider IN ('google', 'github')),
    provider_user_id TEXT NOT NULL,
    created_at       TEXT NOT NULL
);

CREATE UNIQUE INDEX ux_oauth_identities_provider_account ON user_oauth_identities (provider, provider_user_id);
CREATE UNIQUE INDEX ux_oauth_identities_user_provider ON user_oauth_identities (user_id, provider);

-- client_secret is deliberately not a column: resolved separately
-- through internal/secrets, keyed by OAuthProviderSecretsKey(provider).
-- enabled defaults to 0, so an upgrade changes nothing until an operator
-- opts in.
CREATE TABLE oauth_provider_settings (
    provider             TEXT PRIMARY KEY CHECK (provider IN ('google', 'github')),
    enabled              INTEGER NOT NULL DEFAULT 0,
    client_id            TEXT,
    allowed_email_domain TEXT
);

INSERT INTO oauth_provider_settings (provider, enabled) VALUES ('google', 0);
INSERT INTO oauth_provider_settings (provider, enabled) VALUES ('github', 0);

-- Migrates a pre-existing admin_user row into users as the first user.
-- username becomes email verbatim, even without "@", so an upgrading
-- operator keeps signing in with the identifier they already use.
-- admin_user itself is left in place, just no longer read by any Go code.
INSERT INTO users (id, email, display_name, password_hash, is_first_user, created_at, last_login_at)
SELECT 'user_legacy_admin', username, username, password_hash, 1, updated_at, NULL
FROM admin_user WHERE id = 1;
