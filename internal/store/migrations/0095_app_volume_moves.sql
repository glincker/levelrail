-- app_volume_moves tracks "move this app to another node, taking its
-- volumes with it" attempts (internal/backup.MoveVolume, internal/api's
-- handleMoveAppWithVolumes): unlike a plain PUT .../node placement change,
-- this is a multi-step, non-instant operation (stop, archive each volume,
-- restore each onto the new node, flip node_id), so each attempt gets its
-- own row an operator can poll for outcome instead of a fire-and-forget
-- 200. steps is a JSON array of {name, status, error, started_at,
-- finished_at} objects, appended to as each step starts and finishes, the
-- same "structured blob column" shape deploy_attempts.config_snapshot
-- already uses (migrations/0086) for a similarly free-form per-attempt
-- record.
CREATE TABLE app_volume_moves (
    id           TEXT PRIMARY KEY,
    service_name TEXT NOT NULL,
    from_node_id TEXT NOT NULL,
    to_node_id   TEXT NOT NULL,
    status       TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    error        TEXT NOT NULL DEFAULT '',
    steps        TEXT NOT NULL DEFAULT '[]',
    started_at   TEXT NOT NULL,
    finished_at  TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_app_volume_moves_service ON app_volume_moves(service_name, started_at DESC);
