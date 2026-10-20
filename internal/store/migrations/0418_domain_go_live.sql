-- Automatic DNS on domain add: the apps base domain (<app>.<base> for apps
-- created without a domain), record knobs, and the records Levelrail itself
-- created so removal never touches a foreign record.
ALTER TABLE ingress_settings ADD COLUMN apps_base_domain TEXT;
ALTER TABLE ingress_settings ADD COLUMN dns_cname_target TEXT;
ALTER TABLE ingress_settings ADD COLUMN dns_ttl_seconds INTEGER NOT NULL DEFAULT 0 CHECK (dns_ttl_seconds BETWEEN 0 AND 86400);
ALTER TABLE ingress_settings ADD COLUMN dns_proxied INTEGER NOT NULL DEFAULT 0;

-- JSON policy object for the go-live automation (api.domainAutomationStored).
ALTER TABLE ingress_settings ADD COLUMN domain_automation TEXT;

CREATE TABLE domain_automation_runs (
    id         TEXT PRIMARY KEY,
    app_name   TEXT NOT NULL,
    domain     TEXT NOT NULL,
    result     TEXT NOT NULL,
    steps      TEXT NOT NULL,
    undo       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    undone_at  TEXT
);
CREATE INDEX domain_automation_runs_created ON domain_automation_runs (created_at DESC);

CREATE TABLE managed_dns_records (
    domain      TEXT NOT NULL,
    provider    TEXT NOT NULL,
    zone        TEXT NOT NULL,
    name        TEXT NOT NULL,
    record_type TEXT NOT NULL,
    value       TEXT NOT NULL,
    app_name    TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    PRIMARY KEY (domain, record_type)
);
