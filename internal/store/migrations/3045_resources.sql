-- Append-only change log behind the v2 list and watch API. AUTOINCREMENT so a
-- compacted version number is never handed out again.
CREATE TABLE resource_changes (
	version INTEGER PRIMARY KEY AUTOINCREMENT,
	kind TEXT NOT NULL,
	namespace TEXT NOT NULL DEFAULT '',
	name TEXT NOT NULL,
	op TEXT NOT NULL CHECK (op IN ('ADDED', 'MODIFIED', 'DELETED')),
	object TEXT NOT NULL DEFAULT '{}',
	at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX idx_resource_changes_kind_name ON resource_changes (kind, name, version);
CREATE INDEX idx_resource_changes_at ON resource_changes (at);

-- Watermark below which history is incomplete; a watch from before it gets 410.
CREATE TABLE resource_compaction (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	compacted_through INTEGER NOT NULL DEFAULT 0
);
INSERT INTO resource_compaction (id, compacted_through) VALUES (1, 0);
