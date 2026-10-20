-- Per-app environment protection: explicit sensitive/plain choices per
-- variable (JSON object of key -> bool) and the owner's write-only lock.
CREATE TABLE app_env_settings (
    app_name            TEXT PRIMARY KEY REFERENCES desired_services(name) ON DELETE CASCADE,
    sensitive_overrides TEXT NOT NULL DEFAULT '{}',
    secrets_write_only  INTEGER NOT NULL DEFAULT 0,
    updated_at          TEXT NOT NULL
);
