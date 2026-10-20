-- Per-app attachments of internal/integrations' curated catalog
-- (PostHog, Sentry, etc). This table records only which integration is
-- attached to which app; the field values (DSN, API key) go through
-- service_secrets/service_secret_values (migrations/0006) under
-- store.AppIntegrationSecretsKey(service_name, integration_key), the
-- same "reuse the envelope-encryption store, don't build a parallel
-- one" precedent shared-env secrets (migrations/0110) already set.
CREATE TABLE app_integrations (
    id              TEXT PRIMARY KEY,
    service_name    TEXT NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE,
    integration_key TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    UNIQUE (service_name, integration_key)
);

CREATE INDEX idx_app_integrations_service_name ON app_integrations(service_name);
