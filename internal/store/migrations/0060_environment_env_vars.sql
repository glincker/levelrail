-- Shared env vars at the environment level (migrations/0055), the tier
-- between project_env_vars (migrations/0040) and a service's own env:
-- applied after the project layer and before the service's own env,
-- mirroring organization_env_vars' (migrations/0058) own "plain values
-- only" scope note.
CREATE TABLE environment_env_vars (
    environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    key            TEXT NOT NULL,
    value          TEXT NOT NULL,
    updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    PRIMARY KEY (environment_id, key)
);
