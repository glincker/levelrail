-- Forgot-password reset tokens: only the SHA-256 hash is ever persisted
-- (the same convention api_tokens.token_hash establishes). used_at is
-- set once, at successful reset, so a replayed token is rejected rather
-- than deleted outright. user_id names which user's password a given
-- token can reset, since this platform is multi-user.
CREATE TABLE password_reset_tokens (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at    TEXT
);

CREATE INDEX idx_password_reset_tokens_token_hash ON password_reset_tokens(token_hash);
