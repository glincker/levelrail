-- The WebAuthn BE/BS flags (migrations/0255's user_passkeys) were never
-- persisted, so every login compared the live assertion's Backup
-- Eligible flag against a hardcoded false and failed for any synced or
-- hybrid passkey. See internal/passkey.FromWebAuthnCredential.
ALTER TABLE user_passkeys ADD COLUMN backup_eligible INTEGER NOT NULL DEFAULT 0;
ALTER TABLE user_passkeys ADD COLUMN backup_state INTEGER NOT NULL DEFAULT 0;
