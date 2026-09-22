-- Point-in-time restore (PITR) for managed databases: Postgres only
-- today (internal/reconcile/database's own doc comment explains why).
-- pitr_enabled is the opt-in flag SetDatabasePITR writes, off by
-- default like every other opt-in feature toggle in this codebase.
-- pitr_enabled_at records when it was turned on: a restore can only
-- ever target a timestamp after this moment, since WAL archiving (and
-- so any recoverable history) did not exist before it.
ALTER TABLE desired_databases ADD COLUMN pitr_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE desired_databases ADD COLUMN pitr_enabled_at TEXT NOT NULL DEFAULT '';

-- base_backup_history is the physical-backup counterpart of
-- backup_history: one row per attempt to capture a database's entire
-- data directory (via pg_basebackup --format=tar, which embeds
-- Postgres's own backup_label inside the tar itself) as the base a PITR
-- restore replays archived WAL forward from, up to some later target
-- timestamp. A logical dump in backup_history is only ever restorable
-- to the exact moment it was taken; a succeeded row here is restorable
-- to any timestamp between its own started_at and however far WAL
-- archiving has continuously reached since.
CREATE TABLE base_backup_history (
	id TEXT PRIMARY KEY,
	database_name TEXT NOT NULL,
	target_id TEXT NOT NULL,
	object_key TEXT NOT NULL,
	lsn TEXT NOT NULL DEFAULT '',
	size_bytes INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
	error TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL,
	finished_at TEXT NOT NULL DEFAULT ''
);

-- Every PITR-restore-relevant query filters by database_name and
-- either orders by or filters on started_at (the newest-succeeded-
-- before-a-cutoff lookup RunPITRRestore's validation needs, and the
-- oldest-succeeded lookup the recoverable window's lower bound needs),
-- so both columns belong in the same index rather than database_name
-- alone.
CREATE INDEX idx_base_backup_history_database_started ON base_backup_history(database_name, started_at);

-- pitr_restore_history is restore_history's PITR counterpart, a
-- separate table rather than new columns on restore_history: that
-- table's own backup_history_id column is NOT NULL with a foreign key
-- to backup_history(id), which a PITR restore (sourced from
-- base_backup_history plus an arbitrary target_timestamp, not a single
-- backup_history row) cannot populate. Rebuilding restore_history to
-- relax that constraint (SQLite has no ALTER COLUMN) is real risk for
-- an already-shipped table; a second, purpose-shaped table with its own
-- FK to base_backup_history is additive and carries none of that risk.
CREATE TABLE pitr_restore_history (
	id TEXT PRIMARY KEY,
	database_name TEXT NOT NULL,
	base_backup_history_id TEXT NOT NULL REFERENCES base_backup_history(id),
	target_timestamp TEXT NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
	error TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL,
	finished_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_pitr_restore_history_database_started ON pitr_restore_history(database_name, started_at DESC);
