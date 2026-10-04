-- Short-lived, single-use login links minted by an already-authenticated
-- root-ability caller (session or API token), for browser automation to
-- skip manual sign-in. Same hashed/single-use shape as
-- password_reset_tokens: only the SHA-256 hash is ever persisted.
-- principal_type/principal_id name who minted it; abilities is a
-- snapshot taken at mint time, since a token principal has no live
-- record this could be re-resolved from later, unlike a user session.
CREATE TABLE session_link_tokens (
    id             TEXT PRIMARY KEY,
    principal_type TEXT NOT NULL,
    principal_id   TEXT NOT NULL,
    abilities      TEXT NOT NULL,
    display_name   TEXT NOT NULL,
    token_hash     TEXT NOT NULL UNIQUE,
    created_at     TEXT NOT NULL,
    expires_at     TEXT NOT NULL,
    used_at        TEXT
);

CREATE INDEX idx_session_link_tokens_token_hash ON session_link_tokens(token_hash);
