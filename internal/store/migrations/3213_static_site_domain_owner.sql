-- Static site domains (0015) own per-domain settings too: a static preview
-- is gated by basic auth, which the owner triggers of 0483 and 0510 refused
-- because they only knew app and standalone domains. Each trigger is dropped
-- and recreated with the third owner, and the release triggers now also fire
-- when a static site's domain goes, so a gate never outlives its last owner.

DROP TRIGGER IF EXISTS domain_basic_auth_owned;
DROP TRIGGER IF EXISTS domain_maintenance_owned;
DROP TRIGGER IF EXISTS domain_redirect_owned;
DROP TRIGGER IF EXISTS domain_search_visibility_owned;
DROP TRIGGER IF EXISTS domain_tls_cert_owned;
DROP TRIGGER IF EXISTS domain_traffic_policies_owned;
DROP TRIGGER IF EXISTS domain_waf_owned;
DROP TRIGGER IF EXISTS domain_error_page_owned;
DROP TRIGGER IF EXISTS domain_auth_gate_owned;
DROP TRIGGER IF EXISTS service_domains_release_settings;
DROP TRIGGER IF EXISTS standalone_domains_release_settings;
DROP TRIGGER IF EXISTS service_domains_release_auth_gate;
DROP TRIGGER IF EXISTS standalone_domains_release_auth_gate;

CREATE TRIGGER domain_basic_auth_owned BEFORE INSERT ON domain_basic_auth
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_maintenance_owned BEFORE INSERT ON domain_maintenance
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_redirect_owned BEFORE INSERT ON domain_redirect
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_search_visibility_owned BEFORE INSERT ON domain_search_visibility
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_tls_cert_owned BEFORE INSERT ON domain_tls_cert
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_traffic_policies_owned BEFORE INSERT ON domain_traffic_policies
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_waf_owned BEFORE INSERT ON domain_waf
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_error_page_owned BEFORE INSERT ON domain_error_page
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER domain_auth_gate_owned BEFORE INSERT ON domain_auth_gate
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER service_domains_release_settings AFTER DELETE ON service_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain) AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = OLD.domain) AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = OLD.domain)
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

CREATE TRIGGER service_domains_release_auth_gate AFTER DELETE ON service_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain) AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = OLD.domain) AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = OLD.domain)
BEGIN
	DELETE FROM domain_auth_gate WHERE domain = OLD.domain;
END;

CREATE TRIGGER standalone_domains_release_settings AFTER DELETE ON standalone_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain) AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = OLD.domain) AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = OLD.domain)
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

CREATE TRIGGER standalone_domains_release_auth_gate AFTER DELETE ON standalone_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain) AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = OLD.domain) AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = OLD.domain)
BEGIN
	DELETE FROM domain_auth_gate WHERE domain = OLD.domain;
END;

CREATE TRIGGER static_site_domains_release_settings AFTER DELETE ON static_site_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain) AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = OLD.domain) AND NOT EXISTS (SELECT 1 FROM static_site_domains WHERE domain = OLD.domain)
BEGIN
	DELETE FROM domain_basic_auth WHERE domain = OLD.domain;
	DELETE FROM domain_maintenance WHERE domain = OLD.domain;
	DELETE FROM domain_redirect WHERE domain = OLD.domain;
	DELETE FROM domain_search_visibility WHERE domain = OLD.domain;
	DELETE FROM domain_tls_cert WHERE domain = OLD.domain;
	DELETE FROM domain_traffic_policies WHERE domain = OLD.domain;
	DELETE FROM domain_waf WHERE domain = OLD.domain;
	DELETE FROM domain_error_page WHERE domain = OLD.domain;
	DELETE FROM domain_auth_gate WHERE domain = OLD.domain;
END;
