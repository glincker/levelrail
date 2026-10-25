-- Set once when the setup admin's first login is trusted, so revoking that device never reopens trust on first use.
ALTER TABLE users ADD COLUMN first_login_trust_used INTEGER NOT NULL DEFAULT 0;
