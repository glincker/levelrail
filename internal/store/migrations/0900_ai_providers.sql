-- Multi-provider BYOK assistant: per-provider config (keys live in secrets),
-- an ordered fallback chain on the settings row, and per-call usage metering.
CREATE TABLE ai_providers (
    provider_id     TEXT PRIMARY KEY,
    base_url        TEXT NOT NULL DEFAULT '',
    default_model   TEXT NOT NULL DEFAULT '',
    private_url_ok  INTEGER NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'untested',
    status_detail   TEXT NOT NULL DEFAULT '',
    last_checked_at TEXT NOT NULL DEFAULT '',
    updated_at      TEXT NOT NULL
);

ALTER TABLE ai_assistant_settings ADD COLUMN fallback TEXT NOT NULL DEFAULT '[]';

CREATE TABLE ai_usage (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id        TEXT NOT NULL DEFAULT '',
    message_id        TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL,
    provider          TEXT NOT NULL,
    model             TEXT NOT NULL,
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    latency_ms        INTEGER NOT NULL DEFAULT 0,
    error_class       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_ai_usage_created ON ai_usage (created_at);
CREATE INDEX idx_ai_usage_session ON ai_usage (session_id);
