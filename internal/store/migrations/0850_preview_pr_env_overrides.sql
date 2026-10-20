-- Per-pull-request env var overrides for preview environments. Applied on top
-- of the app's own env, preview-env and branch-env overrides when that one PR's
-- preview deploys, and deleted with the preview. Secret values live encrypted
-- under store.PRPreviewEnvSecretsKey, never in this table.
CREATE TABLE preview_pr_env_overrides (
    id TEXT PRIMARY KEY,
    service_name TEXT NOT NULL,
    pr_number INTEGER NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    is_secret INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL,
    UNIQUE (service_name, pr_number, key)
);

CREATE INDEX idx_preview_pr_env_overrides_pr ON preview_pr_env_overrides (service_name, pr_number);

-- Volumes a preview teardown still has to remove. Recorded before the preview's
-- services are deleted so a retry after a partial failure can find them.
CREATE TABLE preview_pending_volumes (
    preview_app_id TEXT NOT NULL,
    volume_name TEXT NOT NULL,
    PRIMARY KEY (preview_app_id, volume_name)
);
