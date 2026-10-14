-- SAML 2.0 sign-in: IdP connections, linked identities, single-use AuthnRequest
-- IDs bound to a browser, seen assertion IDs for replay refusal, and the group
-- to role mappings SAML and SCIM share.
CREATE TABLE saml_connections (
	id               TEXT PRIMARY KEY,
	name             TEXT NOT NULL UNIQUE,
	enabled          INTEGER NOT NULL DEFAULT 0,
	idp_entity_id    TEXT NOT NULL DEFAULT '',
	idp_sso_url      TEXT NOT NULL DEFAULT '',
	idp_certificates TEXT NOT NULL DEFAULT '',
	idp_metadata_url TEXT NOT NULL DEFAULT '',
	email_attribute  TEXT NOT NULL DEFAULT '',
	name_attribute   TEXT NOT NULL DEFAULT '',
	groups_attribute TEXT NOT NULL DEFAULT '',
	allowed_domains  TEXT NOT NULL DEFAULT '[]',
	jit_provisioning INTEGER NOT NULL DEFAULT 0,
	default_role_id  TEXT NOT NULL DEFAULT '',
	sync_roles       INTEGER NOT NULL DEFAULT 0,
	created_at       TEXT NOT NULL,
	updated_at       TEXT NOT NULL
);

CREATE TABLE saml_identities (
	connection_id TEXT NOT NULL REFERENCES saml_connections(id) ON DELETE CASCADE,
	name_id       TEXT NOT NULL,
	user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at    TEXT NOT NULL,
	last_login_at TEXT,
	PRIMARY KEY (connection_id, name_id)
);

CREATE INDEX idx_saml_identities_user ON saml_identities (user_id);

CREATE TABLE saml_authn_requests (
	id            TEXT PRIMARY KEY,
	connection_id TEXT NOT NULL REFERENCES saml_connections(id) ON DELETE CASCADE,
	browser_hash  TEXT NOT NULL,
	expires_at    INTEGER NOT NULL
);

CREATE INDEX idx_saml_authn_requests_browser ON saml_authn_requests (connection_id, browser_hash);

CREATE TABLE saml_seen_assertions (
	connection_id TEXT NOT NULL,
	assertion_id  TEXT NOT NULL,
	expires_at    INTEGER NOT NULL,
	PRIMARY KEY (connection_id, assertion_id)
);

CREATE TABLE sso_group_role_mappings (
	group_name TEXT NOT NULL PRIMARY KEY COLLATE NOCASE,
	role_id    TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
	priority   INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE sso_settings (
	id          INTEGER PRIMARY KEY CHECK (id = 1),
	sp_cert_pem TEXT NOT NULL DEFAULT '',
	enforce_sso INTEGER NOT NULL DEFAULT 0,
	updated_at  TEXT NOT NULL DEFAULT ''
);

INSERT INTO sso_settings (id) VALUES (1);
