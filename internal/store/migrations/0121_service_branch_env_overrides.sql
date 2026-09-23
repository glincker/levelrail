-- Branch-scoped preview env var overrides: a narrower, matched-by-
-- pattern sibling of preview_env_overrides (migrations/0098), which
-- applies to every preview unconditionally. A row here only takes
-- effect when the preview's own branch matches branch_pattern (exact
-- name or a shell glob, "release/*"). is_secret follows the same split
-- migrations/0110 already established for shared env vars: a
-- secret-marked row's value stays '', its plaintext lives in
-- service_secret_values under store.BranchEnvOverrideSecretsKey.
CREATE TABLE service_branch_env_overrides (
    id TEXT PRIMARY KEY,
    service_name TEXT NOT NULL,
    branch_pattern TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    is_secret INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL,
    UNIQUE (service_name, branch_pattern, key)
);

CREATE INDEX idx_service_branch_env_overrides_service_name ON service_branch_env_overrides (service_name);
