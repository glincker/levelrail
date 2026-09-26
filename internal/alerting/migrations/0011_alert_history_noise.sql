-- One row per alert state change or notification decision, so an operator
-- can see what fired, what was suppressed and why. Pruned by age.
CREATE TABLE alert_history (
    id          TEXT PRIMARY KEY,
    at          TEXT NOT NULL,
    rule_id     TEXT NOT NULL,
    rule_name   TEXT NOT NULL,
    rule_kind   TEXT NOT NULL,
    resource_id TEXT NOT NULL DEFAULT '',
    app         TEXT NOT NULL DEFAULT '',
    node        TEXT NOT NULL DEFAULT '',
    severity    TEXT NOT NULL DEFAULT '',
    event       TEXT NOT NULL,   -- fired | resolved | flapping | flap_ended
    outcome     TEXT NOT NULL,   -- sent | silenced | grouped | inhibited | failed | ratelimited | flapping | skipped
    detail      TEXT NOT NULL DEFAULT '',
    silence_id  TEXT NOT NULL DEFAULT '',
    channel_id  TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_alert_history_at ON alert_history (at DESC);
CREATE INDEX idx_alert_history_app_at ON alert_history (app, at DESC);
CREATE INDEX idx_alert_history_rule_at ON alert_history (rule_id, at DESC);

-- Per-rule noise control. Zero values fall back to the control plane defaults.
ALTER TABLE alert_rules ADD COLUMN severity TEXT NOT NULL DEFAULT 'warning';
ALTER TABLE alert_rules ADD COLUMN labels_json TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_rules ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0;
ALTER TABLE alert_rules ADD COLUMN flap_threshold INTEGER NOT NULL DEFAULT 0;
ALTER TABLE alert_rules ADD COLUMN flap_window_seconds INTEGER NOT NULL DEFAULT 0;
