-- Per-domain auth gate (forward auth): every request is checked against an
-- operator-run auth service before reaching the backend. Owned and released
-- like the other per-domain settings (0483). copy_headers is a JSON array.
CREATE TABLE domain_auth_gate (
	domain          TEXT PRIMARY KEY,
	enabled         INTEGER NOT NULL DEFAULT 0,
	auth_url        TEXT NOT NULL,
	check_path      TEXT NOT NULL DEFAULT '/',
	copy_headers    TEXT NOT NULL DEFAULT '[]',
	timeout_seconds INTEGER NOT NULL DEFAULT 0,
	updated_at      TEXT NOT NULL
);

CREATE TRIGGER domain_auth_gate_owned BEFORE INSERT ON domain_auth_gate
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = NEW.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = NEW.domain)
BEGIN
	SELECT RAISE(ABORT, 'FOREIGN KEY constraint failed: domain has no owner');
END;

CREATE TRIGGER service_domains_release_auth_gate AFTER DELETE ON service_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain)
	AND NOT EXISTS (SELECT 1 FROM standalone_domains WHERE domain = OLD.domain)
BEGIN
	DELETE FROM domain_auth_gate WHERE domain = OLD.domain;
END;

CREATE TRIGGER standalone_domains_release_auth_gate AFTER DELETE ON standalone_domains
WHEN NOT EXISTS (SELECT 1 FROM service_domains WHERE domain = OLD.domain)
BEGIN
	DELETE FROM domain_auth_gate WHERE domain = OLD.domain;
END;
