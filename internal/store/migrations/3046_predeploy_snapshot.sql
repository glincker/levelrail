-- Per-app opt-in for a database snapshot taken before each deploy.
CREATE TABLE app_predeploy_snapshot_settings (
	app TEXT PRIMARY KEY,
	mode TEXT NOT NULL DEFAULT 'off' CHECK (mode IN ('off', 'on', 'require')),
	target_id TEXT NOT NULL DEFAULT ''
);

-- One row per database snapshotted ahead of a deploy; deploy_id is filled in
-- once the attempt exists. The restore_* columns track rollback-with-data.
CREATE TABLE deploy_db_snapshots (
	id TEXT PRIMARY KEY,
	app TEXT NOT NULL,
	deploy_id TEXT NOT NULL DEFAULT '',
	prior_deploy_id TEXT NOT NULL DEFAULT '',
	database_name TEXT NOT NULL,
	engine TEXT NOT NULL DEFAULT '',
	backup_history_id TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	error TEXT NOT NULL DEFAULT '',
	size_bytes INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	restored_database TEXT NOT NULL DEFAULT '',
	restore_status TEXT NOT NULL DEFAULT '',
	restore_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_deploy_db_snapshots_app ON deploy_db_snapshots (app, created_at);
CREATE INDEX idx_deploy_db_snapshots_deploy ON deploy_db_snapshots (deploy_id);
