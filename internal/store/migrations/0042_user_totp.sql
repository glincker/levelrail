-- Per-user TOTP two-factor auth (RFC 6238). The secret itself is not a
-- column here: it goes through internal/secrets the same way the GitHub
-- App connection's private key does (migrations/0034), keyed by
-- store.UserTOTPSecretsKey(userID). totp_confirmed_at distinguishes a
-- real enabled state from a setup call that was never confirmed with a
-- valid code; a leftover unconfirmed secret with totp_enabled=0 is
-- inert and gets silently overwritten by the next setup call.
ALTER TABLE users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN totp_confirmed_at TEXT NULL;

-- Single-use fallback codes issued once, in full, at confirm time (and
-- again on regeneration): code_hash is a SHA-256 digest of the
-- plaintext code, the same hashing internal/api's own API tokens use
-- (hashToken), not bcrypt: these are high-entropy (80 random bits)
-- single-use codes looked up by exact hash match, not low-entropy
-- secrets that need per-value salting.
CREATE TABLE user_recovery_codes (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	code_hash TEXT NOT NULL,
	used_at TEXT NULL,
	created_at TEXT NOT NULL
);

CREATE INDEX idx_user_recovery_codes_user_id ON user_recovery_codes(user_id);
CREATE UNIQUE INDEX ux_user_recovery_codes_user_id_code_hash ON user_recovery_codes(user_id, code_hash);
