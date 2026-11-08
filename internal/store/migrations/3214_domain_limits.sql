-- Per-domain request limits: a request body cap and upstream read and write
-- timeouts. 0 means no per-domain value, so the app and instance limits
-- apply. Owned and released like the other per-domain settings, with all
-- three owner tables (3213).
CREATE TABLE domain_limits (
	domain                TEXT PRIMARY KEY,
	max_body_bytes        INTEGER NOT NULL DEFAULT 0,
	read_timeout_seconds  INTEGER NOT NULL DEFAULT 0,
	write_timeout_seconds INTEGER NOT NULL DEFAULT 0,
	updated_at            TEXT NOT NULL
);

CREATE TRIGGER domain_limits_owned BEFORE INSERT ON domain_limits
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER service_domains_release_limits AFTER DELETE ON service_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = OLD.domain)
BEGIN
	DELETE FROM domain_limits WHERE domain = OLD.domain;
END;

CREATE TRIGGER standalone_domains_release_limits AFTER DELETE ON standalone_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = OLD.domain)
BEGIN
	DELETE FROM domain_limits WHERE domain = OLD.domain;
END;

CREATE TRIGGER static_site_domains_release_limits AFTER DELETE ON static_site_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = OLD.domain)
BEGIN
	DELETE FROM domain_limits WHERE domain = OLD.domain;
END;
