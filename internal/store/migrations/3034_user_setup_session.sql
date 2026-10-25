-- The session and address the setup admin registered from, so only that device can claim trust on first use.
ALTER TABLE users ADD COLUMN setup_session_id TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN setup_ip TEXT NOT NULL DEFAULT '';
