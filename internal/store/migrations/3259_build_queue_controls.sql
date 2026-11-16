-- Build queue pause and drain state, one row, plus a row per change for the audit trail.
-- Additive: older binaries never read it and keep building.
CREATE TABLE build_queue_state (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  state      TEXT NOT NULL DEFAULT 'running',
  reason     TEXT NOT NULL DEFAULT '',
  actor      TEXT NOT NULL DEFAULT '',
  since      TEXT NOT NULL DEFAULT ''
);
INSERT INTO build_queue_state (id, state) VALUES (1, 'running');

CREATE TABLE build_queue_events (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  state      TEXT NOT NULL,
  reason     TEXT NOT NULL DEFAULT '',
  actor      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX build_queue_events_created ON build_queue_events (created_at DESC);
