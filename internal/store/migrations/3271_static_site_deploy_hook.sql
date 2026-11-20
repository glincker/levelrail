-- Secret deploy hook for a git-connected static site: only the SHA-256 of the URL secret is kept.
ALTER TABLE static_site_settings ADD COLUMN deploy_hook_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE static_site_settings ADD COLUMN deploy_hook_created_at TEXT NOT NULL DEFAULT '';
