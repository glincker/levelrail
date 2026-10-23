-- Free wildcard-DNS provider for automatic app hostnames: an instance default
-- and per-app overrides.
CREATE TABLE dns_provider_default (
    id                 INTEGER PRIMARY KEY CHECK (id = 1),
    provider           TEXT NOT NULL DEFAULT 'sslip.io',
    custom_base_domain TEXT NOT NULL DEFAULT ''
);
INSERT INTO dns_provider_default (id) VALUES (1);

CREATE TABLE app_dns_provider (
    app_name           TEXT PRIMARY KEY,
    provider           TEXT NOT NULL,
    custom_base_domain TEXT NOT NULL DEFAULT ''
);
