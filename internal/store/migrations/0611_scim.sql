-- SCIM 2.0 provisioning: hashed bearer tokens, directory metadata for users,
-- directory groups with members, and disabled accounts. A disabled row refuses
-- every session and token of the user until it is removed.
CREATE TABLE scim_tokens (
	id           TEXT PRIMARY KEY,
	name         TEXT NOT NULL,
	token_hash   TEXT NOT NULL UNIQUE,
	prefix       TEXT NOT NULL,
	created_by   TEXT NOT NULL DEFAULT '',
	created_at   TEXT NOT NULL,
	last_used_at TEXT,
	revoked_at   TEXT
);

CREATE TABLE scim_users (
	user_id     TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	external_id TEXT NOT NULL DEFAULT '',
	user_name   TEXT NOT NULL,
	given_name  TEXT NOT NULL DEFAULT '',
	family_name TEXT NOT NULL DEFAULT '',
	deleted     INTEGER NOT NULL DEFAULT 0,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);

CREATE INDEX idx_scim_users_external ON scim_users (external_id);

CREATE TABLE scim_groups (
	id           TEXT PRIMARY KEY,
	display_name TEXT NOT NULL UNIQUE COLLATE NOCASE,
	external_id  TEXT NOT NULL DEFAULT '',
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL
);

CREATE TABLE scim_group_members (
	group_id TEXT NOT NULL REFERENCES scim_groups(id) ON DELETE CASCADE,
	user_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	PRIMARY KEY (group_id, user_id)
);

CREATE INDEX idx_scim_group_members_user ON scim_group_members (user_id);

CREATE TABLE disabled_users (
	user_id     TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
	reason      TEXT NOT NULL,
	disabled_at TEXT NOT NULL
);

ALTER TABLE sso_settings ADD COLUMN scim_default_role_id TEXT NOT NULL DEFAULT 'role_viewer';
