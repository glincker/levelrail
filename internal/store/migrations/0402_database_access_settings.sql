CREATE TABLE IF NOT EXISTS database_access_settings (
    database_name TEXT PRIMARY KEY,
    network_scope TEXT NOT NULL DEFAULT 'platform',
    require_tls   INTEGER NOT NULL DEFAULT 0,
    updated_at    TEXT NOT NULL
);
