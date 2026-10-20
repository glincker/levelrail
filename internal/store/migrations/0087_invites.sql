-- Team invites: an authenticated root caller names an email and a role
-- (or hand-picked abilities), the platform mints a token and only ever
-- persists its hash (same convention as password_reset_tokens.token_hash
-- and api_tokens.token_hash). Accepting the invite creates exactly the
-- one user it named, through the same path POST /api/v1/auth/users uses.
-- created_by names the inviting user, nullable since an API token (not
-- backed by a users row) can also create one; ON DELETE SET NULL so
-- deleting that admin later doesn't cascade into deleting invites they
-- happened to send.
CREATE TABLE invites (
    id          TEXT PRIMARY KEY,
    email       TEXT NOT NULL,
    role        TEXT NOT NULL DEFAULT '',
    abilities   TEXT NOT NULL,
    token_hash  TEXT NOT NULL UNIQUE,
    created_by  TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TEXT NOT NULL,
    expires_at  TEXT NOT NULL,
    accepted_at TEXT,
    revoked_at  TEXT
);

CREATE INDEX idx_invites_token_hash ON invites(token_hash);

-- Partial unique index (same shape as ux_users_single_first_user,
-- migrations/0035): at most one outstanding, unresolved invite per
-- email, so a second create for the same address is rejected rather
-- than silently piling up tokens for one address.
CREATE UNIQUE INDEX ux_invites_pending_email ON invites(email) WHERE accepted_at IS NULL AND revoked_at IS NULL;
