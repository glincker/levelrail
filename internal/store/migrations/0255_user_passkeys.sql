-- WebAuthn (passkey) credentials per user. The public key is COSE-encoded
-- key material, the public half of an asymmetric keypair, not a secret:
-- unlike a TOTP secret (migrations/0042) it does not go through
-- internal/secrets, the same way a TLS certificate's public half is
-- stored in the clear.
--
-- credential_id is the WebAuthn credential ID the authenticator returns
-- on every ceremony, raw bytes stored base64url-encoded like every other
-- opaque token column in this schema; sign_count guards against a cloned
-- authenticator (internal/passkey bumps it on every successful login and
-- rejects a non-increasing value).
CREATE TABLE user_passkeys (
	id             TEXT PRIMARY KEY,
	user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	credential_id  TEXT NOT NULL,
	public_key     BLOB NOT NULL,
	sign_count     INTEGER NOT NULL DEFAULT 0,
	aaguid         TEXT NOT NULL DEFAULT '',
	transports     TEXT NOT NULL DEFAULT '',
	label          TEXT NOT NULL,
	created_at     TEXT NOT NULL,
	last_used_at   TEXT NULL
);

CREATE INDEX idx_user_passkeys_user_id ON user_passkeys(user_id);
CREATE UNIQUE INDEX ux_user_passkeys_credential_id ON user_passkeys(credential_id);
