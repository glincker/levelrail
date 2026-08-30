-- Per-domain HTTP Basic Auth: an operator can protect any domain a
-- service currently claims (service_domains, 0012) with a
-- username/password, enforced by Caddy's basic_auth handler
-- (internal/reconcile/ingress). Only the username lives here; the
-- password goes through internal/secrets under
-- store.DomainBasicAuthSecretsKey(domain), the same envelope-encryption
-- split store.CloudflareTunnelSettings already uses for its own token.
-- Foreign key CASCADE means removing a domain from an app (which
-- deletes its service_domains row via claimServiceDomains) also drops
-- any basic auth configured for it, so a stale row can never outlive
-- the domain it protects.
CREATE TABLE domain_basic_auth (
	domain   TEXT PRIMARY KEY REFERENCES service_domains(domain) ON DELETE CASCADE,
	username TEXT NOT NULL
);
