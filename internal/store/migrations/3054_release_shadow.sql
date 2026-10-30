-- Immutable release snapshots minted from succeeded deploy attempts (ADR 050, shadow phase).
CREATE TABLE releases (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL DEFAULT '',
	source_app TEXT NOT NULL,
	source_deploy_attempt_id TEXT NOT NULL UNIQUE,
	image_ref TEXT NOT NULL,
	image_tag TEXT NOT NULL DEFAULT '',
	config TEXT NOT NULL DEFAULT '{}',
	commit_sha TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	created_by TEXT NOT NULL DEFAULT ''
);

CREATE INDEX releases_source_app ON releases (source_app, created_at);

-- What each app's binding would point at; nothing reads it for decisions yet.
CREATE TABLE environment_bindings (
	app TEXT PRIMARY KEY,
	environment_id TEXT NOT NULL DEFAULT '',
	release_id TEXT,
	state TEXT NOT NULL DEFAULT 'active',
	previous_release_id TEXT,
	updated_at TEXT NOT NULL,
	updated_by TEXT NOT NULL DEFAULT ''
);

-- Append only.
CREATE TABLE environment_binding_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	app TEXT NOT NULL,
	release_id TEXT NOT NULL,
	changed_at TEXT NOT NULL,
	changed_by TEXT NOT NULL DEFAULT '',
	reason TEXT NOT NULL DEFAULT '',
	approval_id TEXT NOT NULL DEFAULT ''
);

CREATE INDEX environment_binding_history_app ON environment_binding_history (app, id);
