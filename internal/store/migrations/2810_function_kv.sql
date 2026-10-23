-- Per-function key-value store. The namespace is the function (service) name and is part of every key lookup.
CREATE TABLE function_kv (
    namespace  TEXT NOT NULL,
    key        TEXT NOT NULL,
    value      BLOB NOT NULL,
    expires_at INTEGER,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (namespace, key)
);

CREATE INDEX function_kv_expiry ON function_kv (expires_at) WHERE expires_at IS NOT NULL;

-- Lookup hash of each function's KV bearer token. The token itself lives encrypted with the function's other secrets.
CREATE TABLE function_kv_tokens (
    namespace  TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    rotated_at INTEGER NOT NULL
);
