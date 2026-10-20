-- Guarded major version upgrades of managed Postgres databases: one row per
-- attempt. snapshot_volume names the untouched copy of the pre-upgrade data
-- volume kept as the rollback target until an operator discards it.
CREATE TABLE database_major_upgrades (
	id TEXT PRIMARY KEY,
	database_name TEXT NOT NULL,
	from_version TEXT NOT NULL,
	to_version TEXT NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'rolled_back')),
	phase TEXT NOT NULL DEFAULT '',
	snapshot_volume TEXT NOT NULL DEFAULT '',
	error TEXT NOT NULL DEFAULT '',
	started_at TEXT NOT NULL,
	finished_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_database_major_upgrades_db ON database_major_upgrades (database_name, started_at DESC);
