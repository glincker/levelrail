CREATE TABLE IF NOT EXISTS plugin_trusted_keys (
    key_id     TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    public_key TEXT NOT NULL,
    added_at   TEXT NOT NULL,
    added_by   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS plugins (
    name                TEXT PRIMARY KEY,
    version             TEXT NOT NULL,
    kind                TEXT NOT NULL,
    publisher           TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    image               TEXT NOT NULL,
    digest              TEXT NOT NULL,
    manifest            TEXT NOT NULL,
    signature           TEXT NOT NULL,
    key_id              TEXT NOT NULL,
    requested_abilities TEXT NOT NULL DEFAULT '[]',
    approved_abilities  TEXT NOT NULL DEFAULT '[]',
    outbound_hosts      TEXT NOT NULL DEFAULT '[]',
    config_json         TEXT NOT NULL DEFAULT '{}',
    state               TEXT NOT NULL,
    approved_at         TEXT NOT NULL DEFAULT '',
    approved_by         TEXT NOT NULL DEFAULT '',
    installed_at        TEXT NOT NULL,
    installed_by        TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    last_error          TEXT NOT NULL DEFAULT ''
);
