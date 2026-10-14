-- Per-team and per-project resource limits. Zero in any column means
-- unlimited, so a missing row and an all-zero row behave the same.
CREATE TABLE resource_quotas (
    scope            TEXT NOT NULL,
    scope_id         TEXT NOT NULL,
    max_apps         INTEGER NOT NULL DEFAULT 0,
    max_databases    INTEGER NOT NULL DEFAULT 0,
    max_memory_bytes INTEGER NOT NULL DEFAULT 0,
    max_nano_cpus    INTEGER NOT NULL DEFAULT 0,
    max_previews     INTEGER NOT NULL DEFAULT 0,
    updated_at       TEXT NOT NULL,
    PRIMARY KEY (scope, scope_id)
);
