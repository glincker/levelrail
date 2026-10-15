-- SAML and SCIM now run on the auth library (0750). These tables keep only
-- what the library has no place for: connection names, enable switches, the
-- JIT and domain policy, IdP fields of a connection still being set up, the
-- group to role mappings and the enforcement switch. The interim tables of
-- 0610 and 0611 (never released) are dropped where a dev database has them.
DROP TABLE IF EXISTS saml_identities;
DROP TABLE IF EXISTS saml_authn_requests;
DROP TABLE IF EXISTS saml_seen_assertions;
DROP TABLE IF EXISTS saml_connections;
DROP TABLE IF EXISTS scim_group_members;
DROP TABLE IF EXISTS scim_groups;
DROP TABLE IF EXISTS scim_users;
DROP TABLE IF EXISTS scim_tokens;
DROP TABLE IF EXISTS disabled_users;
DROP TABLE IF EXISTS sso_group_role_mappings;
DROP TABLE IF EXISTS sso_settings;

CREATE TABLE sso_saml_connections (
	id                    TEXT PRIMARY KEY,
	library_id            TEXT UNIQUE,
	name                  TEXT NOT NULL UNIQUE,
	enabled               INTEGER NOT NULL DEFAULT 0,
	idp_metadata_url      TEXT NOT NULL DEFAULT '',
	allowed_domains       TEXT NOT NULL DEFAULT '[]',
	jit_provisioning      INTEGER NOT NULL DEFAULT 0,
	default_role_id       TEXT NOT NULL DEFAULT '',
	sync_roles            INTEGER NOT NULL DEFAULT 0,
	draft_idp_entity_id   TEXT NOT NULL DEFAULT '',
	draft_idp_sso_url     TEXT NOT NULL DEFAULT '',
	draft_idp_certificate TEXT NOT NULL DEFAULT '',
	draft_email_attribute TEXT NOT NULL DEFAULT '',
	draft_name_attribute  TEXT NOT NULL DEFAULT '',
	draft_groups_attribute TEXT NOT NULL DEFAULT '',
	created_at            TEXT NOT NULL,
	updated_at            TEXT NOT NULL
);

CREATE TABLE sso_group_role_mappings (
	group_name TEXT NOT NULL PRIMARY KEY COLLATE NOCASE,
	role_id    TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
	priority   INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE sso_settings (
	id                   INTEGER PRIMARY KEY CHECK (id = 1),
	enforce_sso          INTEGER NOT NULL DEFAULT 0,
	scim_default_role_id TEXT NOT NULL DEFAULT 'role_viewer',
	updated_at           TEXT NOT NULL DEFAULT ''
);

INSERT INTO sso_settings (id) VALUES (1);

-- Display metadata for library SCIM tokens; the token hash lives in the library.
CREATE TABLE sso_scim_token_meta (
	token_id   TEXT PRIMARY KEY,
	prefix     TEXT NOT NULL DEFAULT '',
	created_by TEXT NOT NULL DEFAULT ''
);
