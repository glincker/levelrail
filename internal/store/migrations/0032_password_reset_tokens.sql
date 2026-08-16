-- Forgot-password reset tokens: a short-lived, single-use,
-- cryptographically random token whose SHA-256 hash (never the raw
-- token) is persisted, the same hashing convention api_tokens.token_hash
-- already establishes for bearer API tokens (internal/api's hashToken).
--
-- used_at is nullable and set exactly once, at successful reset: a
-- second attempt with the same token is rejected because used_at is no
-- longer NULL, not because the row is deleted, so a reused token is
-- diagnosable from this table rather than looking identical to one that
-- never existed.
CREATE TABLE password_reset_tokens (
    id         TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at    TEXT
);

CREATE INDEX idx_password_reset_tokens_token_hash ON password_reset_tokens(token_hash);
