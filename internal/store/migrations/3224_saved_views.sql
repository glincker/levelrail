-- Named list filters per owner; a team shared view is readable by members of team_id only.
CREATE TABLE saved_views (
	id         TEXT PRIMARY KEY,
	owner      TEXT NOT NULL,
	team_id    TEXT NOT NULL DEFAULT '',
	list       TEXT NOT NULL,
	name       TEXT NOT NULL,
	filter     TEXT NOT NULL DEFAULT '{}',
	shared     TEXT NOT NULL DEFAULT 'private' CHECK (shared IN ('private', 'team')),
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE (owner, list, name)
);

CREATE INDEX saved_views_team ON saved_views (team_id, shared, list);

-- Triggers catch every delete and revoke path, so a reused id never inherits old views.
CREATE TRIGGER users_drop_saved_views AFTER DELETE ON users
BEGIN
	DELETE FROM saved_views WHERE owner = 'user:' || OLD.id;
END;

CREATE TRIGGER api_tokens_drop_saved_views AFTER DELETE ON api_tokens
BEGIN
	DELETE FROM saved_views WHERE owner = 'token:' || OLD.id;
END;

CREATE TRIGGER api_tokens_revoke_saved_views AFTER UPDATE OF revoked_at ON api_tokens
WHEN NEW.revoked_at IS NOT NULL
BEGIN
	DELETE FROM saved_views WHERE owner = 'token:' || NEW.id;
END;
