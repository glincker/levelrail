-- App types (ADR 052, slice 1): one editable draft per type plus immutable published versions.
CREATE TABLE app_types (
	name TEXT PRIMARY KEY,
	draft TEXT NOT NULL,
	updated_by TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);

CREATE TABLE app_type_versions (
	name TEXT NOT NULL,
	version INTEGER NOT NULL,
	document TEXT NOT NULL,
	digest TEXT NOT NULL,
	published_by TEXT NOT NULL DEFAULT '',
	published_at TEXT NOT NULL,
	PRIMARY KEY (name, version)
);
