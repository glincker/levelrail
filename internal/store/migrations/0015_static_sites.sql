-- Phase 1 gap close: TASKS.md 1.4/1.6's own "build.type: static needs
-- ingress integration first" note, now closed. Static sites are the
-- architecture's one deliberate exception to the desired_services shape
-- (migrations/0002): "static sites get served by the embedded Caddy
-- directly with no container" (CLAUDE.md 4.4). There is no image, no
-- port, no container to converge, so a static site's desired state does
-- not belong in desired_services at all: it would force every reader of
-- that table (the application controller, telemetry/log target
-- collection in cmd/levelrail, the ingress controller's own container
-- lookup) to grow a container/no-container branch for a resource type
-- that, by design, bypasses the reconcile-to-a-running-container model
-- entirely. A separate table keeps that bypass honest instead of
-- threading a special case through code that has no other reason to
-- know static sites exist.
--
-- static_sites.root_dir is a local filesystem path (internal/deploy
-- copies a git checkout's static output here, under the control plane's
-- data directory) that internal/reconcile/ingress's controller reads on
-- every reconcile pass and points Caddy's file_server handler at
-- directly: no port, no Docker inspection, no container name derivation.
--
-- static_site_domains mirrors service_domains (migrations/0012)
-- exactly, as its own child table rather than reusing service_domains
-- directly: service_domains.service_name has a NOT NULL FOREIGN KEY
-- REFERENCES desired_services(name), and a static site is deliberately
-- never a row in desired_services (see above), so it cannot be a valid
-- service_domains.service_name either. Two separate child tables, one
-- shared cross-table uniqueness check at claim time (store.checkDomainOwner,
-- used by both claimServiceDomains in service.go and
-- claimStaticSiteDomains in static_site.go), is what keeps a container
-- service and a static site from ever claiming the same domain, closing
-- the same class of bug migrations/0012's own comment describes, now for
-- a second desired-state table instead of just one.
CREATE TABLE static_sites (
	name       TEXT PRIMARY KEY,
	domains    TEXT NOT NULL DEFAULT '[]',
	root_dir   TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE static_site_domains (
	domain            TEXT PRIMARY KEY,
	static_site_name  TEXT NOT NULL REFERENCES static_sites(name) ON DELETE CASCADE
);

CREATE INDEX idx_static_site_domains_name ON static_site_domains(static_site_name);
