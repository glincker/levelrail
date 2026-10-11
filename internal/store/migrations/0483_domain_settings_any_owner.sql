-- Per-domain settings (basic auth, maintenance, BYO certificate, WAF,
-- redirect, error pages, search visibility, traffic policies) referenced
-- service_domains, so only app domains could carry them. Standalone
-- domains (0481) need the same settings, so each table is rebuilt without
-- the foreign key. Triggers keep both old guarantees: an insert needs an
-- owner (app or standalone), and a domain's settings go when its last
-- owner releases it.

CREATE TABLE domain_basic_auth_new (
	domain   TEXT PRIMARY KEY,
	username TEXT NOT NULL
);
INSERT INTO domain_basic_auth_new (domain, username) SELECT domain, username FROM domain_basic_auth;
DROP TABLE domain_basic_auth;
ALTER TABLE domain_basic_auth_new RENAME TO domain_basic_auth;

CREATE TABLE domain_maintenance_new (
	domain TEXT PRIMARY KEY
);
INSERT INTO domain_maintenance_new (domain) SELECT domain FROM domain_maintenance;
DROP TABLE domain_maintenance;
ALTER TABLE domain_maintenance_new RENAME TO domain_maintenance;

CREATE TABLE domain_redirect_new (
	domain      TEXT PRIMARY KEY,
	target_url  TEXT NOT NULL,
	status_code INTEGER NOT NULL DEFAULT 301
);
INSERT INTO domain_redirect_new (domain, target_url, status_code) SELECT domain, target_url, status_code FROM domain_redirect;
DROP TABLE domain_redirect;
ALTER TABLE domain_redirect_new RENAME TO domain_redirect;

CREATE TABLE domain_search_visibility_new (
	domain TEXT PRIMARY KEY
);
INSERT INTO domain_search_visibility_new (domain) SELECT domain FROM domain_search_visibility;
DROP TABLE domain_search_visibility;
ALTER TABLE domain_search_visibility_new RENAME TO domain_search_visibility;

CREATE TABLE domain_tls_cert_new (
	domain      TEXT PRIMARY KEY,
	uploaded_at TEXT NOT NULL,
	expires_at  TEXT NOT NULL
);
INSERT INTO domain_tls_cert_new (domain, uploaded_at, expires_at) SELECT domain, uploaded_at, expires_at FROM domain_tls_cert;
DROP TABLE domain_tls_cert;
ALTER TABLE domain_tls_cert_new RENAME TO domain_tls_cert;

CREATE TABLE domain_traffic_policies_new (
	domain     TEXT NOT NULL,
	kind       TEXT NOT NULL CHECK (kind IN ('headers', 'forwarders', 'geo', 'cache', 'redirects')),
	spec       TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (domain, kind)
);
INSERT INTO domain_traffic_policies_new (domain, kind, spec, updated_at) SELECT domain, kind, spec, updated_at FROM domain_traffic_policies;
DROP TABLE domain_traffic_policies;
ALTER TABLE domain_traffic_policies_new RENAME TO domain_traffic_policies;

CREATE TABLE domain_waf_new (
	domain           TEXT PRIMARY KEY,
	waf_enabled      INTEGER NOT NULL DEFAULT 0,
	waf_mode         TEXT NOT NULL DEFAULT 'detect',
	rate_limit_rps   INTEGER NOT NULL DEFAULT 0,
	rate_limit_burst INTEGER NOT NULL DEFAULT 0
);
INSERT INTO domain_waf_new (domain, waf_enabled, waf_mode, rate_limit_rps, rate_limit_burst)
SELECT domain, waf_enabled, waf_mode, rate_limit_rps, rate_limit_burst FROM domain_waf;
DROP TABLE domain_waf;
ALTER TABLE domain_waf_new RENAME TO domain_waf;

CREATE TABLE domain_error_page_new (
	domain      TEXT NOT NULL,
	status_code INTEGER NOT NULL,
	body        TEXT NOT NULL,
	PRIMARY KEY (domain, status_code)
);
INSERT INTO domain_error_page_new (domain, status_code, body) SELECT domain, status_code, body FROM domain_error_page;
DROP TABLE domain_error_page;
ALTER TABLE domain_error_page_new RENAME TO domain_error_page;

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

CREATE TRIGGER domain_basic_auth_owned BEFORE INSERT ON domain_basic_auth
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_maintenance_owned BEFORE INSERT ON domain_maintenance
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_redirect_owned BEFORE INSERT ON domain_redirect
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_search_visibility_owned BEFORE INSERT ON domain_search_visibility
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_tls_cert_owned BEFORE INSERT ON domain_tls_cert
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_traffic_policies_owned BEFORE INSERT ON domain_traffic_policies
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_waf_owned BEFORE INSERT ON domain_waf
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_error_page_owned BEFORE INSERT ON domain_error_page
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;
