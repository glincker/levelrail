-- Dashboard-edited settings and routing rules for a static site. They live
-- beside static_sites so a redeploy that rewrites that row never drops them.
CREATE TABLE IF NOT EXISTS static_site_settings (
    name       TEXT PRIMARY KEY,
    settings   TEXT NOT NULL DEFAULT '{}',
    ui_rules   TEXT NOT NULL DEFAULT '{}',
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
