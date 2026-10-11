-- Per-domain traffic controls (headers, forwarders, geo, cache, redirects),
-- one JSON row per domain and kind, validated by internal/trafficpolicy.
-- No row means the default behavior; deleting a row restores it.
CREATE TABLE domain_traffic_policies (
    domain     TEXT NOT NULL REFERENCES service_domains(domain) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('headers', 'forwarders', 'geo', 'cache', 'redirects')),
    spec       TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (domain, kind)
);
