-- Extends users with the same Abilities scoping api_tokens already has
-- (0007_api_tokens.sql): requireAbility's session branch now checks a
-- user's own abilities instead of treating every session as implicitly
-- root. Every user created before this migration has always had
-- unconditional full access, so both the column default and the
-- explicit backfill below land on '["root"]'; downgrading that
-- silently on upgrade would lock existing operators out of their own
-- control plane.
ALTER TABLE users ADD COLUMN abilities TEXT NOT NULL DEFAULT '["root"]';

UPDATE users SET abilities = '["root"]';
