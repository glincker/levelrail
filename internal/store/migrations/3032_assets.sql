-- Index of objects brokered through levelrail; the provider listing stays the source of truth.
CREATE TABLE IF NOT EXISTS assets (
	bucket_id    TEXT NOT NULL REFERENCES storage_buckets (id) ON DELETE CASCADE,
	key          TEXT NOT NULL,
	size_bytes   INTEGER NOT NULL DEFAULT 0,
	content_type TEXT NOT NULL DEFAULT '',
	etag         TEXT NOT NULL DEFAULT '',
	uploader     TEXT NOT NULL DEFAULT '',
	created_at   TEXT NOT NULL,
	last_seen_at TEXT NOT NULL,
	PRIMARY KEY (bucket_id, key)
);
