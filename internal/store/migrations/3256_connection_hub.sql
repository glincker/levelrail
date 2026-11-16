-- Sidecar for the integrations hub read model. Existing connection tables stay authoritative;
-- a missing row means instance scope and unknown health, so older binaries ignore it.
CREATE TABLE connection_meta (
  ref           TEXT PRIMARY KEY,
  scope_type    TEXT NOT NULL DEFAULT 'instance' CHECK (scope_type IN ('instance', 'org', 'app')),
  scope_id      TEXT NOT NULL DEFAULT '',
  last_used_at  TEXT NOT NULL DEFAULT '',
  expires_at    TEXT NOT NULL DEFAULT '',
  rotate_after  TEXT NOT NULL DEFAULT '',
  health        TEXT NOT NULL DEFAULT 'unknown',
  health_detail TEXT NOT NULL DEFAULT '',
  checked_at    TEXT NOT NULL DEFAULT '',
  updated_at    TEXT NOT NULL
);

-- Links from a connection ref to an app, stack or org with per-role rules (JSON).
CREATE TABLE connection_bindings (
  id          TEXT PRIMARY KEY,
  ref         TEXT NOT NULL,
  target_type TEXT NOT NULL CHECK (target_type IN ('app', 'stack', 'org')),
  target_id   TEXT NOT NULL,
  role        TEXT NOT NULL,
  rules       TEXT NOT NULL DEFAULT '{}',
  created_by  TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL,
  UNIQUE (ref, target_type, target_id, role)
);
CREATE INDEX connection_bindings_target ON connection_bindings (target_type, target_id);
