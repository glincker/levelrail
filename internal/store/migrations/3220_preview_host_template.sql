-- Per-app preview hostname: a template such as pr-{number}.{domain} rendered
-- under a wildcard domain the app already owns. Empty keeps the default host.
ALTER TABLE preview_app_settings ADD COLUMN host_template TEXT NOT NULL DEFAULT '';
ALTER TABLE preview_app_settings ADD COLUMN wildcard_domain TEXT NOT NULL DEFAULT '';
