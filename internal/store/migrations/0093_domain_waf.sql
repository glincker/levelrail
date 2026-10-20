-- Per-domain, opt-in Web Application Firewall (OWASP Coraza) and rate
-- limiting, both driven by Caddy modules already vendored into the
-- embedded ingress binary (internal/ingress, ADR 005): no new container,
-- no new infra dependency. Mirrors domain_basic_auth (0053) and
-- domain_maintenance (0080): presence of a row means the domain has
-- customized at least one of these two independent toggles.
--
-- waf_mode: "detect" (default, OWASP CRS runs but only logs, matching
-- Coraza's own documented SecRuleEngine DetectionOnly behavior) or
-- "block" (SecRuleEngine On, CRS matches actually reject the request).
-- Detection-only is the safer default for a security feature an
-- operator is enabling for the first time: it surfaces false positives
-- in logs before anything starts rejecting real traffic.
--
-- rate_limit_rps = 0 means rate limiting is off for this domain, the
-- same "0 is the off state" convention this schema already uses
-- elsewhere (see backup_targets.retain_count's own doc comment for the
-- same shape applied to a different field).
CREATE TABLE domain_waf (
    domain           TEXT PRIMARY KEY REFERENCES service_domains(domain) ON DELETE CASCADE,
    waf_enabled      INTEGER NOT NULL DEFAULT 0,
    waf_mode         TEXT NOT NULL DEFAULT 'detect',
    rate_limit_rps   INTEGER NOT NULL DEFAULT 0,
    rate_limit_burst INTEGER NOT NULL DEFAULT 0
);
