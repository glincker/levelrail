-- Highest TOTP time step already accepted for a user: a code is single-use,
-- so a replayed or shoulder-surfed code inside its validity window is refused.
ALTER TABLE users ADD COLUMN totp_last_step INTEGER NOT NULL DEFAULT 0;
