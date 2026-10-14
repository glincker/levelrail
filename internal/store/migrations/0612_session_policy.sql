-- Per-team session and access policy, enforced by the API middleware. A user
-- in several teams gets the strictest combination. session_activity tracks
-- idle time and how each session signed in.
CREATE TABLE session_policies (
	team_id              TEXT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
	idle_timeout_seconds INTEGER NOT NULL DEFAULT 0,
	max_lifetime_seconds INTEGER NOT NULL DEFAULT 0,
	ip_allowlist         TEXT NOT NULL DEFAULT '[]',
	require_mfa          INTEGER NOT NULL DEFAULT 0,
	updated_by           TEXT NOT NULL DEFAULT '',
	updated_at           TEXT NOT NULL
);

CREATE TABLE session_activity (
	session_id   TEXT PRIMARY KEY,
	user_id      TEXT NOT NULL,
	method       TEXT NOT NULL DEFAULT '',
	last_seen_at INTEGER NOT NULL
);

CREATE INDEX idx_session_activity_user ON session_activity (user_id);
