-- Server-side apply field ownership: which manager last set each spec leaf of a v2 resource.
CREATE TABLE resource_managed_fields (
	kind TEXT NOT NULL,
	name TEXT NOT NULL,
	path TEXT NOT NULL,
	manager TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	PRIMARY KEY (kind, name, path, manager)
);
