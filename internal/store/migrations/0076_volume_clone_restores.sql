-- volume_clone_restores tracks "restore as new volume" attempts: the app
-- service volume counterpart of clone_restores (migrations/0074). Each row
-- names the source service/volume a backup was taken from and a brand-new,
-- standalone Docker volume name it was restored into, never touching the
-- source volume's own contents. Unlike a database clone-restore, the new
-- volume is not a desired-state resource of its own (app service volumes
-- only exist as entries in desired_services' own JSON blob, see
-- store.ServiceVolume), so this table is the only record of the new
-- volume's existence and lineage.
CREATE TABLE volume_clone_restores (
    id                  TEXT PRIMARY KEY,
    source_service_name TEXT NOT NULL,
    source_volume_name  TEXT NOT NULL,
    new_volume_name     TEXT NOT NULL,
    backup_history_id   TEXT NOT NULL REFERENCES backup_history(id),
    status              TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    error               TEXT NOT NULL DEFAULT '',
    started_at          TEXT NOT NULL,
    finished_at         TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_volume_clone_restores_source ON volume_clone_restores(source_service_name, source_volume_name, started_at DESC);
