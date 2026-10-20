-- One row per deploy-outcome notify target, scoped to one app
-- (resource_id), the same per-app scoping alert_rules.resource_id
-- already uses. Deliberately its own table rather than a new
-- kind='deploy_outcome' row in alert_rules: a deploy-outcome
-- notification fires exactly once, synchronously, the moment a
-- deploy_attempts row reaches a terminal status (see
-- internal/alerting/deploy_notify.go's own doc comment), it is never
-- evaluated on Engine's tick the way every alert_rules row is. Giving it
-- alert_rules' columns would mean either leaving
-- metric/comparator/threshold/restart_count_threshold/restart_window/
-- pending_since/firing/firing_since permanently at their zero value (a
-- polymorphic-row shape that only made sense in the first place because
-- threshold and crashloop really do share one "evaluate on a tick, track
-- firing state" model) or teaching Engine.Tick a third, evaluation-free
-- code path just to leave every one of those columns alone. A second,
-- much narrower table is the smaller, clearer change.
CREATE TABLE deploy_notify_targets (
    id          TEXT PRIMARY KEY,
    resource_id TEXT NOT NULL, -- e.g. "service:web", matches alert_rules.resource_id's own convention

    notify_url  TEXT NOT NULL DEFAULT '',
    notify_kind TEXT NOT NULL DEFAULT 'generic', -- 'generic' | 'slack' | 'discord' | 'telegram' | 'email'

    enabled     INTEGER NOT NULL DEFAULT 1,

    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE INDEX idx_deploy_notify_targets_resource ON deploy_notify_targets (resource_id);
