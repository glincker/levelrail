-- Which app instance owns a named app volume, so a same-name app created later cannot silently mount it.
CREATE TABLE app_volume_owners (
    volume_name TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL
);
