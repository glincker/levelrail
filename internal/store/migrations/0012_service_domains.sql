-- Phase 3 (TASKS.md 3.6): closes the domain-uniqueness gap
-- internal/reconcile/ingress/controller.go's own package doc comment has
-- flagged as open since Phase 1 (internal/spec's Validate() only ever
-- sees one app.yaml at a time, so nothing stopped two separate deploys
-- from each claiming the same domain; BuildRoutesConfig would then
-- silently build two Caddy routes matching the same Host header, and
-- whichever sorted last inside ingress.RoutesOptions.Routes would win
-- inside Caddy's own matcher evaluation, silently shadowing the other).
--
-- service_domains is a child table (one row per domain) rather than a
-- UNIQUE constraint on some column of desired_services directly,
-- because desired_services.domains (0004) is a single JSON-encoded
-- column, one row per service, not one row per domain: SQLite has no
-- native way to put a uniqueness constraint across values nested inside
-- a JSON column. store.SaveDesiredService (internal/store/service.go)
-- keeps this table in sync with desired_services.domains on every
-- write, claiming each domain inside the same transaction and failing
-- the whole write with ErrDomainTaken if a domain is already claimed by
-- a different service.
--
-- Backfill: any domain already claimed by more than one existing
-- service before this migration (only possible pre-3.6, since nothing
-- enforced uniqueness until now) is resolved by keeping the
-- alphabetically-first service name and silently dropping the rest
-- (INSERT OR IGNORE on the domain PRIMARY KEY); this is an arbitrary
-- but deterministic tie-break for data that was already an unnoticed
-- bug, not a new correctness regression, since the pre-migration schema
-- had no owner information for these domains at all. The next redeploy
-- of the domain-losing service surfaces ErrDomainTaken, which is the
-- first time this platform has ever surfaced that conflict to an
-- operator at all.
CREATE TABLE service_domains (
	domain       TEXT PRIMARY KEY,
	service_name TEXT NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE
);

CREATE INDEX idx_service_domains_service_name ON service_domains(service_name);

INSERT OR IGNORE INTO service_domains (domain, service_name)
SELECT je.value, ds.name
FROM desired_services ds, json_each(ds.domains) je
WHERE je.value IS NOT NULL AND je.value != ''
ORDER BY ds.name, je.value;
