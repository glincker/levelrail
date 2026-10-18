-- Ingress firewall: ordered rules plus one settings row. Rules hold
-- operator-written conditions only, never request data.
CREATE TABLE IF NOT EXISTS edge_firewall_rules (
	id                  TEXT PRIMARY KEY,
	name                TEXT NOT NULL,
	note                TEXT NOT NULL DEFAULT '',
	scope               TEXT NOT NULL DEFAULT '',
	enabled             INTEGER NOT NULL DEFAULT 1,
	position            INTEGER NOT NULL,
	action              TEXT NOT NULL CHECK (action IN ('allow', 'deny', 'challenge', 'log', 'rate_limit')),
	conditions          TEXT NOT NULL,
	rate_count          INTEGER NOT NULL DEFAULT 0,
	rate_window_seconds INTEGER NOT NULL DEFAULT 0,
	created_at          TEXT NOT NULL,
	updated_at          TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS edge_firewall_rules_position ON edge_firewall_rules (position, id);

CREATE TABLE IF NOT EXISTS edge_firewall_settings (
	id       INTEGER PRIMARY KEY CHECK (id = 1),
	settings TEXT NOT NULL
);

INSERT OR IGNORE INTO edge_firewall_settings (id, settings)
VALUES (1, '{"bot_mode":"off","ai_bot_mode":"off","ai_bot_agents":[],"bans_enabled":true,"attack_challenge":false}');
