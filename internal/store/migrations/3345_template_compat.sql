-- Local record of why a template could not install on this host, one row per (template, node arch, class).
-- Cleared when the same template installs on that arch. Never uploaded.
CREATE TABLE IF NOT EXISTS template_compat (
    template_id TEXT NOT NULL,
    node_arch   TEXT NOT NULL,
    class       TEXT NOT NULL,
    detail      TEXT NOT NULL DEFAULT '',
    failures    INTEGER NOT NULL DEFAULT 1,
    first_seen  TEXT NOT NULL,
    last_seen   TEXT NOT NULL,
    PRIMARY KEY (template_id, node_arch, class)
);
