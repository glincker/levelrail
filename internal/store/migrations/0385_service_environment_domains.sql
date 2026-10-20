-- Per-environment domain sets. desired_services.domains stays the app's
-- default set; a row here overrides it while the app's environment_id
-- matches. Every domain is still claimed in service_domains (0012), so
-- hostname uniqueness holds across all sets.
CREATE TABLE service_environment_domains (
    service_name   TEXT NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE,
    environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    domain         TEXT NOT NULL,
    PRIMARY KEY (service_name, environment_id, domain)
);

CREATE INDEX idx_service_environment_domains_env ON service_environment_domains (environment_id);
