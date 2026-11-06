-- One row per sampled moment of data dir free space and per-category Docker bytes, for the storage trend and forecast.
CREATE TABLE disk_history_samples (
    sampled_at         TEXT PRIMARY KEY,
    free_bytes         INTEGER NOT NULL DEFAULT 0,
    total_bytes        INTEGER NOT NULL DEFAULT 0,
    images_bytes       INTEGER NOT NULL DEFAULT 0,
    containers_bytes   INTEGER NOT NULL DEFAULT 0,
    volumes_bytes      INTEGER NOT NULL DEFAULT 0,
    build_cache_bytes  INTEGER NOT NULL DEFAULT 0
);
