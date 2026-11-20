-- Optional shared password for a static site: a bcrypt hash, empty when the site is open.
ALTER TABLE static_site_settings ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';
