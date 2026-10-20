-- Feature flags: a boolean (plus optional gradual rollout percentage) an
-- app's own running code reads live via GET /api/v1/flags/evaluate/{key},
-- the same self-hosted LaunchDarkly/Unleash-style callback model, not a
-- baked-in env var (env vars are fixed at container-create time, the
-- whole reason this table exists instead of just adding another env
-- var). Scoped to an owning app the same way scheduled_tasks
-- (0048_scheduled_tasks.sql) scopes to service_name, for CRUD nesting
-- and dashboard grouping.
--
-- key is globally unique, not just unique within service_name: the
-- evaluate endpoint is deliberately flat (no app name in its URL)
-- because the API token an app presents is a fleet-wide ability grant
-- (internal/store/tokens.go's own APIToken has no app scoping), so
-- there is no app name available to disambiguate a lookup by key alone.
CREATE TABLE feature_flags (
    id                  TEXT PRIMARY KEY,
    key                 TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    service_name        TEXT NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE,
    enabled             INTEGER NOT NULL DEFAULT 0,
    rollout_percentage  INTEGER NOT NULL DEFAULT 100 CHECK (rollout_percentage BETWEEN 0 AND 100),
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE INDEX idx_feature_flags_service_name ON feature_flags(service_name);
