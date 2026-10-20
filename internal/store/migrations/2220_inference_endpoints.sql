-- External inference endpoints: model servers Levelrail does not run (Ollama,
-- LM Studio, an MLX server, a cloud API). One row per models row whose
-- compute is 'external'. The optional credential lives in the secrets manager
-- under store.ModelSecretsKey(name). fallback_model is empty when the routing
-- policy is off and fallback_model_ref is the model id sent to it; latency_budget_ms 0 means no budget.
CREATE TABLE inference_endpoints (
    name                 TEXT PRIMARY KEY,
    base_url             TEXT NOT NULL,
    api_type             TEXT NOT NULL,
    credential_set       INTEGER NOT NULL DEFAULT 0,
    fallback_model       TEXT NOT NULL DEFAULT '',
    fallback_model_ref   TEXT NOT NULL DEFAULT '',
    latency_budget_ms    INTEGER NOT NULL DEFAULT 0,
    health_state         TEXT NOT NULL DEFAULT 'unknown',
    health_latency_ms    INTEGER NOT NULL DEFAULT 0,
    health_error         TEXT NOT NULL DEFAULT '',
    health_checked_at    TEXT NOT NULL DEFAULT '',
    last_ok_at           TEXT NOT NULL DEFAULT '',
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    discovered_models    TEXT NOT NULL DEFAULT '[]',
    discovery_error      TEXT NOT NULL DEFAULT '',
    fallback_count       INTEGER NOT NULL DEFAULT 0,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);
