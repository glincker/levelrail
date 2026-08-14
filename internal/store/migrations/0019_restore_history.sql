-- restore_history is a append-only record of attempted restores, the
-- same "keep every attempt, success or failure" reasoning
-- migrations/0018_backup_targets.sql's own comment gives for
-- backup_history: a restore is the single most destructive operation
-- this platform exposes (it overwrites a live database's actual data),
-- so an operator being able to look back and see exactly which restore
-- ran, from which backup, when, and whether it succeeded, is not
-- optional the way it might be for a lower-stakes action.
--
-- A separate table from backup_history rather than a direction flag
-- added to it: a restore attempt's shape genuinely differs (it names a
-- source backup_history row instead of a target_id/object_key pair, and
-- it has no size_bytes, since nothing is uploaded), and overloading one
-- table with two different row shapes distinguished only by a flag would
-- make every query and every future column addition ask "does this
-- apply to both directions or just one" indefinitely. backup_history_id
-- makes the link explicit and queryable in the direction that actually
-- matters (which restores came from which backup), which a flag on a
-- shared table could not do without a second self-referential column
-- anyway.
CREATE TABLE restore_history (
    id                TEXT PRIMARY KEY,
    database_name     TEXT NOT NULL,
    backup_history_id TEXT NOT NULL REFERENCES backup_history(id),
    status            TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    error             TEXT NOT NULL DEFAULT '',
    started_at        TEXT NOT NULL,
    finished_at       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_restore_history_database_name ON restore_history(database_name, started_at DESC);
