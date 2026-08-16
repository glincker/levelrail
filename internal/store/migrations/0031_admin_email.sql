-- Adds an optional recovery email address to the single admin account,
-- for the forgot-password flow. Empty string (the DEFAULT every admin
-- bootstrapped before this migration gets) is a real, valid "no
-- recovery email on file" state, not an error: see
-- internal/api/password_reset.go's own handling of that case.
ALTER TABLE admin_user ADD COLUMN email TEXT NOT NULL DEFAULT '';
