-- Per-domain proxy tuning: compression, streaming flush, websocket stream
-- lifetimes and upstream timeouts. Only bounded numbers and flags, never raw
-- proxy config. Released with the domain like domain_limits (3214).
CREATE TABLE domain_tuning (
	domain                          TEXT PRIMARY KEY,
	preset                          TEXT NOT NULL DEFAULT '',
	compression                     INTEGER NOT NULL DEFAULT 0,
	streaming                       INTEGER NOT NULL DEFAULT 0,
	websocket                       INTEGER NOT NULL DEFAULT 0,
	dial_timeout_seconds            INTEGER NOT NULL DEFAULT 0,
	response_header_timeout_seconds INTEGER NOT NULL DEFAULT 0,
	stream_timeout_seconds          INTEGER NOT NULL DEFAULT 0,
	stream_close_delay_seconds      INTEGER NOT NULL DEFAULT 0,
	updated_at                      TEXT NOT NULL
);
CREATE TRIGGER domain_tuning_owned BEFORE INSERT ON domain_tuning
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER service_domains_release_tuning AFTER DELETE ON service_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = OLD.domain)
BEGIN
	DELETE FROM domain_tuning WHERE domain = OLD.domain;
END;

CREATE TRIGGER standalone_domains_release_tuning AFTER DELETE ON standalone_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = OLD.domain)
BEGIN
	DELETE FROM domain_tuning WHERE domain = OLD.domain;
END;

CREATE TRIGGER static_site_domains_release_tuning AFTER DELETE ON static_site_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = OLD.domain)
BEGIN
	DELETE FROM domain_tuning WHERE domain = OLD.domain;
END;
