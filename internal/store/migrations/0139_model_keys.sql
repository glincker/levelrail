-- Named virtual API keys per model. Only the SHA-256 of a key is stored.
-- Empty expires_at, revoked_at and last_used_at mean never, not revoked and
-- unused; a limit of 0 means unlimited. replaced_by is set on the old key
-- of a rotation, which keeps working until its expires_at grace deadline.
-- allow_paths and allow_models are JSON string arrays, empty meaning any.
CREATE TABLE model_keys (
    id             TEXT PRIMARY KEY,
    model_name     TEXT NOT NULL,
    name           TEXT NOT NULL,
    key_hash       TEXT NOT NULL,
    key_prefix     TEXT NOT NULL,
    rpm            INTEGER NOT NULL DEFAULT 0,
    tpm            INTEGER NOT NULL DEFAULT 0,
    max_parallel   INTEGER NOT NULL DEFAULT 0,
    allow_paths    TEXT NOT NULL DEFAULT '[]',
    allow_models   TEXT NOT NULL DEFAULT '[]',
    replaced_by    TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    expires_at     TEXT NOT NULL DEFAULT '',
    revoked_at     TEXT NOT NULL DEFAULT '',
    last_used_at   TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_model_keys_hash ON model_keys(key_hash);
CREATE UNIQUE INDEX idx_model_keys_live_name ON model_keys(model_name, name) WHERE replaced_by = '' AND revoked_at = '';
CREATE INDEX idx_model_keys_model ON model_keys(model_name);

INSERT INTO model_keys (id, model_name, name, key_hash, key_prefix, created_at)
SELECT 'default-' || name, name, 'default', api_key_hash, api_key_prefix, created_at FROM models;
