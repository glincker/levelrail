-- Platform-wide ingress settings (ADR 005's own "Verified" section names
-- this exact gap: real ACME issuance against a public domain was
-- explicitly unproven at the time that ADR was accepted, spot-checked
-- against a real domain before Phase 1's exit criterion could be claimed
-- done, not assumed to follow automatically from the internal-issuer
-- result). This table is the operator-facing toggle that closes it: a
-- global switch between Caddy's offline "internal" issuer (default,
-- byte-identical to every deploy before this table existed) and real
-- ACME (Let's Encrypt or any RFC 8555-compatible CA), plus an optional
-- primary domain for the control plane's own dashboard.
--
-- One authoritative row (id = 1, enforced by the CHECK constraint the
-- same way any other genuinely-singleton settings table would), not a
-- loose key-value table: every column here has a real, fixed type and a
-- real, fixed meaning, matching this schema's general preference for
-- explicit columns over a generic settings blob (see desired_services'
-- own typed-JSON-column choices for the same reasoning applied
-- per-field instead of per-table).
--
-- primary_domain: optional hostname for the control plane's own
-- dashboard (internal/reconcile/ingress adds one more route for it when
-- set, reverse-proxied to the dashboard's own existing bind address).
-- NULL means "not set", the same "absence is a real, permanent state,
-- not merely unset" reasoning desired_services.project_id's own
-- migration comment (0022_projects.sql) already establishes.
--
-- acme_enabled: 0 (off) by default. This is the single most important
-- default in this migration: every existing deployment upgrading into
-- this table gets acme_enabled = 0, so internal/ingress/routes.go's
-- BuildRoutesConfig keeps building the exact same internal-issuer-only
-- automation policy it always has, zero behavior change for anyone who
-- doesn't explicitly opt in.
--
-- acme_email: required by internal/api's PUT /api/v1/settings/ingress
-- validation whenever acme_enabled is set true (Let's Encrypt's own ACME
-- account model requires a contact address), nullable here because it is
-- genuinely optional while acme_enabled stays false.
--
-- acme_directory_url: optional CA directory URL override. Empty/NULL
-- means internal/ingress.NewACMEIssuer leaves Caddy's own compiled-in
-- default in place (Let's Encrypt's real production directory,
-- https://acme-v02.api.letsencrypt.org/directory per
-- caddytls.ACMEIssuer's own doc comment). Sizing note for operators, not
-- enforced here: point this at Let's Encrypt's staging directory
-- (https://acme-staging-v02.api.letsencrypt.org/directory) while
-- testing, since production has real, low, and embarrassing-to-hit rate
-- limits.
CREATE TABLE ingress_settings (
    id                 INTEGER PRIMARY KEY CHECK (id = 1),
    primary_domain     TEXT,
    acme_enabled       INTEGER NOT NULL DEFAULT 0,
    acme_email         TEXT,
    acme_directory_url TEXT
);

INSERT INTO ingress_settings (id, primary_domain, acme_enabled, acme_email, acme_directory_url)
VALUES (1, NULL, 0, NULL, NULL);
