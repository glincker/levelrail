-- Allow the per-domain abuse shield section next to the other traffic controls.
-- The release triggers name the table, so they are dropped and recreated
-- around the rebuild exactly as 0483 defined them.
DROP TRIGGER service_domains_release_settings;
DROP TRIGGER standalone_domains_release_settings;

CREATE TABLE domain_traffic_policies_new (
	domain     TEXT NOT NULL,
	kind       TEXT NOT NULL CHECK (kind IN ('headers', 'forwarders', 'geo', 'cache', 'redirects', 'shield')),
	spec       TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (domain, kind)
);
INSERT INTO domain_traffic_policies_new (domain, kind, spec, updated_at) SELECT domain, kind, spec, updated_at FROM domain_traffic_policies;
DROP TABLE domain_traffic_policies;
ALTER TABLE domain_traffic_policies_new RENAME TO domain_traffic_policies;

CREATE TRIGGER domain_traffic_policies_owned BEFORE INSERT ON domain_traffic_policies
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER service_domains_release_settings AFTER DELETE ON service_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = OLD.domain)
BEGIN
	DELETE FROM domain_basic_auth WHERE domain = OLD.domain;
	DELETE FROM domain_maintenance WHERE domain = OLD.domain;
	DELETE FROM domain_redirect WHERE domain = OLD.domain;
	DELETE FROM domain_search_visibility WHERE domain = OLD.domain;
	DELETE FROM domain_tls_cert WHERE domain = OLD.domain;
	DELETE FROM domain_traffic_policies WHERE domain = OLD.domain;
	DELETE FROM domain_waf WHERE domain = OLD.domain;
	DELETE FROM domain_error_page WHERE domain = OLD.domain;
END;

CREATE TRIGGER standalone_domains_release_settings AFTER DELETE ON standalone_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain)
BEGIN
	DELETE FROM domain_basic_auth WHERE domain = OLD.domain;
	DELETE FROM domain_maintenance WHERE domain = OLD.domain;
	DELETE FROM domain_redirect WHERE domain = OLD.domain;
	DELETE FROM domain_search_visibility WHERE domain = OLD.domain;
	DELETE FROM domain_tls_cert WHERE domain = OLD.domain;
	DELETE FROM domain_traffic_policies WHERE domain = OLD.domain;
	DELETE FROM domain_waf WHERE domain = OLD.domain;
	DELETE FROM domain_error_page WHERE domain = OLD.domain;
END;
