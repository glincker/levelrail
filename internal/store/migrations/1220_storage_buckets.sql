-- Named buckets carved out of a connected storage destination by key prefix,
-- bound to apps by env injection at container start.
CREATE TABLE IF NOT EXISTS storage_buckets (
	id             TEXT PRIMARY KEY,
	name           TEXT NOT NULL UNIQUE,
	target_id      TEXT NOT NULL REFERENCES backup_targets (id),
	prefix         TEXT NOT NULL,
	public         INTEGER NOT NULL DEFAULT 0,
	quota_bytes    INTEGER NOT NULL DEFAULT 0,
	lifecycle_days INTEGER NOT NULL DEFAULT 0,
	last_swept_at  TEXT NOT NULL DEFAULT '',
	created_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS storage_bucket_bindings (
	bucket_id  TEXT NOT NULL REFERENCES storage_buckets (id) ON DELETE CASCADE,
	app_name   TEXT NOT NULL,
	env_prefix TEXT NOT NULL DEFAULT 'S3',
	created_at TEXT NOT NULL,
	PRIMARY KEY (bucket_id, app_name),
	UNIQUE (app_name, env_prefix)
);
