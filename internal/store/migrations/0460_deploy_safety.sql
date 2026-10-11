-- Per-app overrides for the readiness gate and post-deploy watch. A NULL
-- column inherits the APP_DEPLOY_* environment default.
CREATE TABLE deploy_safety_settings (
    service_name        TEXT PRIMARY KEY,
    success_threshold   INTEGER CHECK (success_threshold IS NULL OR success_threshold >= 1),
    stable_window_ms    INTEGER CHECK (stable_window_ms IS NULL OR stable_window_ms >= 0),
    drain_timeout_ms    INTEGER CHECK (drain_timeout_ms IS NULL OR drain_timeout_ms >= 0),
    watch_window_ms     INTEGER CHECK (watch_window_ms IS NULL OR watch_window_ms >= 0),
    watch_max_restarts  INTEGER CHECK (watch_max_restarts IS NULL OR watch_max_restarts >= 1),
    watch_error_rate    REAL CHECK (watch_error_rate IS NULL OR (watch_error_rate > 0 AND watch_error_rate <= 1)),
    watch_min_requests  INTEGER CHECK (watch_min_requests IS NULL OR watch_min_requests >= 1),
    auto_rollback       INTEGER CHECK (auto_rollback IS NULL OR auto_rollback IN (0, 1)),
    updated_at          TEXT NOT NULL
);

-- One row per step a release passed or failed on its way to taking traffic
-- and through its post-deploy watch.
CREATE TABLE deploy_gate_events (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    deploy_attempt_id TEXT NOT NULL,
    service_name      TEXT NOT NULL,
    phase             TEXT NOT NULL,
    status            TEXT NOT NULL,
    reason            TEXT NOT NULL DEFAULT '',
    message           TEXT NOT NULL DEFAULT '',
    container         TEXT NOT NULL DEFAULT '',
    at                TEXT NOT NULL
);

CREATE INDEX deploy_gate_events_attempt ON deploy_gate_events (deploy_attempt_id, at);
CREATE INDEX deploy_gate_events_phase_at ON deploy_gate_events (phase, at);
