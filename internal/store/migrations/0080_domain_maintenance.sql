-- Per-domain maintenance mode: an operator can take any domain a
-- service currently claims (service_domains, 0012) out of rotation,
-- serving a fixed response instead of proxying to the container,
-- without stopping the container itself (internal/reconcile/ingress).
-- Mirrors domain_basic_auth (0053) exactly: presence of a row means
-- enabled, there is no separate boolean to go out of sync with it.
-- Foreign key CASCADE means removing a domain from an app also drops
-- any maintenance-mode row for it, so a stale row can never outlive
-- the domain it was set on.
CREATE TABLE domain_maintenance (
	domain TEXT PRIMARY KEY REFERENCES service_domains(domain) ON DELETE CASCADE
);
