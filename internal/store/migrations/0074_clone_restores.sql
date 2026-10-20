-- clone_restores tracks "restore as new database" attempts: unlike
-- restore_history (an in-place, destructive overwrite of an existing
-- database), each row here names both the source database a backup was
-- taken from and a brand-new database it was restored into, created
-- fresh for this attempt and never touching the source's own data.
--
-- A separate table from restore_history rather than a nullable
-- "new_database_name" column added to it: restore_history's own
-- database_name column means "the database being overwritten", and a
-- clone attempt's target is a database that didn't exist until this
-- attempt created it, a different enough meaning that overloading one
-- column for both would make every future query ask which kind of row
-- it's looking at.
CREATE TABLE clone_restores (
    id                   TEXT PRIMARY KEY,
    source_database_name TEXT NOT NULL,
    new_database_name    TEXT NOT NULL,
    backup_history_id    TEXT NOT NULL REFERENCES backup_history(id),
    status               TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    error                TEXT NOT NULL DEFAULT '',
    started_at           TEXT NOT NULL,
    finished_at          TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_clone_restores_source_database_name ON clone_restores(source_database_name, started_at DESC);
