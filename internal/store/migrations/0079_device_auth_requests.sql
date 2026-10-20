-- Device authorization requests: the CLI-login-from-terminal flow
-- (RFC 8628 shape), where a CLI polls device_code while the operator
-- approves user_code from the web dashboard. status moves
-- pending -> approved|denied, or pending -> expired once past
-- expires_at; approved is terminal once the CLI has actually redeemed
-- it (redeemed_at set), so a device_code can only ever mint one token.
CREATE TABLE device_auth_requests (
	id TEXT PRIMARY KEY,
	device_code TEXT NOT NULL UNIQUE,
	user_code TEXT NOT NULL UNIQUE,
	status TEXT NOT NULL DEFAULT 'pending',
	client_name TEXT NOT NULL DEFAULT '',
	approved_by_user_id TEXT,
	token_id TEXT,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	redeemed_at TEXT
);

CREATE INDEX idx_device_auth_requests_status ON device_auth_requests (status, expires_at);
