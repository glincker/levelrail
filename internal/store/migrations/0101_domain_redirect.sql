-- Per-domain redirect: an operator can point one app-owned domain at an
-- arbitrary absolute target URL (e.g. "redirect www.example.com to
-- example.com", or an old domain to a new app during a migration),
-- enforced by Caddy's static_response handler (internal/reconcile/ingress)
-- instead of proxying to the container. Mirrors domain_maintenance (0080)
-- and domain_waf (0093): presence of a row means the domain has a redirect
-- configured. Foreign key CASCADE means removing a domain from an app also
-- drops any redirect row for it, so a stale row can never outlive the
-- domain it was set on.
--
-- status_code defaults to 301 (permanent), matching Caddy's own default
-- redirect status and the more common real-world case (a domain rename or
-- www-to-apex normalization is rarely reverted); 302 (temporary) is the
-- other supported value, for a redirect an operator expects to undo.
CREATE TABLE domain_redirect (
    domain      TEXT PRIMARY KEY REFERENCES service_domains(domain) ON DELETE CASCADE,
    target_url  TEXT NOT NULL,
    status_code INTEGER NOT NULL DEFAULT 301
);
