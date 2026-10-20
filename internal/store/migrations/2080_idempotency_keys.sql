-- Cached responses for retried mutating API calls, keyed by caller and client-supplied key.
CREATE TABLE idempotency_keys (
    principal    TEXT NOT NULL,
    key          TEXT NOT NULL,
    route        TEXT NOT NULL,
    body_hash    TEXT NOT NULL,
    status       INTEGER NOT NULL,
    content_type TEXT NOT NULL,
    response     BLOB NOT NULL,
    created_at   TEXT NOT NULL,
    PRIMARY KEY (principal, key)
);
CREATE INDEX idx_idempotency_keys_created ON idempotency_keys (created_at);
