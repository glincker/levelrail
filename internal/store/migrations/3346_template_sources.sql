-- Private template sources: a git repo of templates, synced on demand. deploy_key_app names an app
-- whose stored git deploy key is reused; the key itself never lives in this table.
CREATE TABLE IF NOT EXISTS template_sources (
    name           TEXT PRIMARY KEY,
    git_url        TEXT NOT NULL,
    ref            TEXT NOT NULL DEFAULT '',
    deploy_key_app TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL DEFAULT 'pending',
    status_detail  TEXT NOT NULL DEFAULT '',
    last_sync      TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL
);

-- Validated entries of the last good sync. id is the namespaced source/name id.
CREATE TABLE IF NOT EXISTS template_source_entries (
    source        TEXT NOT NULL,
    id            TEXT NOT NULL,
    name          TEXT NOT NULL,
    template_yaml TEXT NOT NULL,
    compose_yaml  TEXT NOT NULL,
    PRIMARY KEY (source, id)
);
