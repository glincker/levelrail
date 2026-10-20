-- The auth library owns second factors, device login and password reset
-- tokens. These in-house tables and columns have no remaining reader.
DROP TABLE IF EXISTS user_recovery_codes;
DROP TABLE IF EXISTS user_passkeys;
DROP TABLE IF EXISTS device_auth_requests;
DROP TABLE IF EXISTS password_reset_tokens;
ALTER TABLE users DROP COLUMN totp_enabled;
ALTER TABLE users DROP COLUMN totp_confirmed_at;
ALTER TABLE users DROP COLUMN totp_last_step;
