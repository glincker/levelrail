-- Built-in sign-in as an auth gate provider: who may pass, single-use
-- handoff tokens, host-bound gate sessions and a compact access log.
-- Released with the gate row (0510); user and team removal cascade.
ALTER TABLE domain_auth_gate ADD COLUMN provider TEXT NOT NULL DEFAULT 'external'
	CHECK (provider IN ('external', 'builtin'));

CREATE TABLE gate_access_rules (
	domain     TEXT NOT NULL REFERENCES domain_auth_gate(domain) ON DELETE CASCADE,
	kind       TEXT NOT NULL CHECK (kind IN ('user', 'team', 'role')),
	subject    TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (domain, kind, subject)
);

CREATE TABLE gate_tokens (
	token_hash        TEXT PRIMARY KEY,
	domain            TEXT NOT NULL,
	user_id           TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	parent_session_id TEXT NOT NULL,
	state_hash        TEXT NOT NULL,
	created_at        TEXT NOT NULL,
	expires_at        TEXT NOT NULL
);
CREATE INDEX gate_tokens_expires_at ON gate_tokens (expires_at);

CREATE TABLE gate_sessions (
	token_hash        TEXT PRIMARY KEY,
	domain            TEXT NOT NULL,
	user_id           TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	parent_session_id TEXT NOT NULL,
	created_at        TEXT NOT NULL,
	expires_at        TEXT NOT NULL,
	revoked_at        TEXT
);
CREATE INDEX gate_sessions_domain ON gate_sessions (domain);
CREATE INDEX gate_sessions_expires_at ON gate_sessions (expires_at);

CREATE TABLE gate_events (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	at         TEXT NOT NULL,
	bucket     TEXT NOT NULL DEFAULT '',
	domain     TEXT NOT NULL,
	user_id    TEXT NOT NULL DEFAULT '',
	user_email TEXT NOT NULL DEFAULT '',
	decision   TEXT NOT NULL,
	reason     TEXT NOT NULL DEFAULT '',
	ip         TEXT NOT NULL DEFAULT '',
	count      INTEGER NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX gate_events_bucket ON gate_events (domain, user_id, decision, reason, ip, bucket);
CREATE INDEX gate_events_domain_at ON gate_events (domain, at);
CREATE INDEX gate_events_at ON gate_events (at);

CREATE TRIGGER domain_auth_gate_release_sessions AFTER DELETE ON domain_auth_gate
BEGIN
	DELETE FROM gate_sessions WHERE domain = OLD.domain;
	DELETE FROM gate_tokens WHERE domain = OLD.domain;
END;

CREATE TRIGGER organizations_release_gate_rules AFTER DELETE ON organizations
BEGIN
	DELETE FROM gate_access_rules WHERE kind = 'team' AND subject = OLD.id;
END;

CREATE TRIGGER users_release_gate_rules AFTER DELETE ON users
BEGIN
	DELETE FROM gate_access_rules WHERE kind = 'user' AND subject = OLD.id;
END;

CREATE TRIGGER roles_release_gate_rules AFTER DELETE ON roles
BEGIN
	DELETE FROM gate_access_rules WHERE kind = 'role' AND subject = OLD.id;
END;
