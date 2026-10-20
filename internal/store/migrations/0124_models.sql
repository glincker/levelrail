-- AI model resources served from a GPU node. node_id '' is the local
-- node, same convention as desired_services. api_key_hash is the SHA-256
-- of the OpenAI-compatible API key, shown to the operator once at
-- creation. The optional HuggingFace token lives in service_secrets under
-- store.ModelSecretsKey(name). deleting=1 is a tombstone the model
-- controller tears down (container, then row).
CREATE TABLE models (
    name            TEXT PRIMARY KEY,
    engine          TEXT NOT NULL,
    model_ref       TEXT NOT NULL,
    node_id         TEXT NOT NULL DEFAULT '',
    gpu_count       INTEGER NOT NULL DEFAULT -1,
    gpu_device_ids  TEXT NOT NULL DEFAULT '[]',
    context_length  INTEGER NOT NULL DEFAULT 0,
    quantization    TEXT NOT NULL DEFAULT '',
    domain          TEXT NOT NULL DEFAULT '',
    api_key_hash    TEXT NOT NULL,
    api_key_prefix  TEXT NOT NULL,
    hf_token_set    INTEGER NOT NULL DEFAULT 0,
    endpoint_dial   TEXT NOT NULL DEFAULT '',
    restart_nonce   INTEGER NOT NULL DEFAULT 0,
    deleting        INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_models_domain ON models(domain) WHERE domain <> '';
CREATE INDEX idx_models_node ON models(node_id);
