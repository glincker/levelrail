-- Pasted Dockerfile sources, kept by content hash so a redeploy or queued build rebuilds the same text without a git host.
CREATE TABLE app_inline_sources (
	service_name TEXT NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE,
	hash         TEXT NOT NULL,
	dockerfile   TEXT NOT NULL,
	files        TEXT NOT NULL DEFAULT '[]',
	size_bytes   INTEGER NOT NULL,
	file_count   INTEGER NOT NULL,
	created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	PRIMARY KEY (service_name, hash)
);
