-- Pipelines v2: durable artifacts, inbound webhook triggers, and the
-- webhook payload recorded on the run that it started.
ALTER TABLE pipeline_runs ADD COLUMN trigger_payload TEXT NOT NULL DEFAULT '';
ALTER TABLE pipeline_runs ADD COLUMN trigger_content_type TEXT NOT NULL DEFAULT '';

-- One stored archive per job that declared artifacts. storage_path is
-- relative to the artifact directory; sha256 is checked on every read.
CREATE TABLE pipeline_artifacts (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES pipeline_runs(id) ON DELETE CASCADE,
    job_key TEXT NOT NULL,
    name TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    sha256 TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    UNIQUE (run_id, job_key)
);

CREATE INDEX idx_pipeline_artifacts_expiry ON pipeline_artifacts (expires_at);

-- At most one inbound webhook trigger per pipeline. Only the sha256 of the
-- URL token is stored. In hmac mode the shared secret lives in the secrets
-- store, never in this table.
CREATE TABLE pipeline_webhook_triggers (
    pipeline_id TEXT PRIMARY KEY REFERENCES pipelines(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    hmac INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    rotated_at TEXT NOT NULL,
    last_used_at TEXT
);

-- Every authenticated delivery, kept for history and replay. The unique
-- index makes a retried delivery with the same idempotency key a no-op.
-- outcome: received, started, rejected, failed.
CREATE TABLE pipeline_webhook_deliveries (
    id TEXT PRIMARY KEY,
    pipeline_id TEXT NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    received_at TEXT NOT NULL,
    status_code INTEGER NOT NULL DEFAULT 0,
    outcome TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    run_id TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT '',
    query TEXT NOT NULL DEFAULT '',
    body BLOB NOT NULL DEFAULT x''
);

CREATE INDEX idx_pipeline_webhook_deliveries_pipeline ON pipeline_webhook_deliveries (pipeline_id, received_at DESC);
CREATE UNIQUE INDEX idx_pipeline_webhook_deliveries_idem ON pipeline_webhook_deliveries (pipeline_id, idempotency_key) WHERE idempotency_key != '';
