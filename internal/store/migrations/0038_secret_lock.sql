-- Reversible per-secret overwrite guard: a locked secret's value can
-- still never be read back (that guarantee doesn't change), but PUT
-- .../secrets/{key} refuses to overwrite it without an explicit
-- confirmation. Unlike Coolify's one-way "shown once" lock, this is
-- meant to be toggled back off, so it's a plain mutable flag, not a
-- write-once marker.
ALTER TABLE service_secret_values ADD COLUMN locked INTEGER NOT NULL DEFAULT 0;
