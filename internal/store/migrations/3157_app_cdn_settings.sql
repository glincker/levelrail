-- Per-app override of the global Cloudflare proxy setting; no row means inherit.
CREATE TABLE app_cdn_settings (
    app_name    TEXT PRIMARY KEY REFERENCES desired_services(name) ON DELETE CASCADE,
    dns_proxied INTEGER NOT NULL,
    updated_at  TEXT NOT NULL,
    updated_by  TEXT NOT NULL DEFAULT ''
);
