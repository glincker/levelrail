-- Per-domain custom error pages: an operator can replace Caddy's bare
-- default error text with their own HTML for a small, fixed set of
-- status codes (404, 500, 502, 503), served by the embedded ingress
-- (internal/ingress) instead of whatever the backend or Caddy itself
-- would otherwise return. Mirrors domain_redirect (0102) and domain_waf
-- (0093) in shape, but unlike those single-row toggles, a domain can
-- have more than one status code configured at once, hence the
-- composite primary key instead of domain alone.
--
-- Allowed status codes are enforced by internal/api, not a CHECK
-- constraint here, matching domain_waf.waf_mode's own enum validation
-- being API-side rather than SQL-side.
CREATE TABLE domain_error_page (
    domain      TEXT NOT NULL REFERENCES service_domains(domain) ON DELETE CASCADE,
    status_code INTEGER NOT NULL,
    body        TEXT NOT NULL,
    PRIMARY KEY (domain, status_code)
);
