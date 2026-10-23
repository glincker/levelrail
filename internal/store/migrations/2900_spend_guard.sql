-- Spend caps gain a deploy guard mode (alert only, or refuse new deploys at
-- 100%) and a root-granted, expiring override that is kept as an audit trail.
ALTER TABLE app_spend_caps ADD COLUMN guard_mode TEXT NOT NULL DEFAULT 'alert' CHECK (guard_mode IN ('alert', 'block_deploys'));
ALTER TABLE cost_budgets ADD COLUMN guard_mode TEXT NOT NULL DEFAULT 'alert' CHECK (guard_mode IN ('alert', 'block_deploys'));

CREATE TABLE spend_guard_overrides (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    app_name   TEXT NOT NULL,
    period     TEXT NOT NULL,
    reason     TEXT NOT NULL,
    actor      TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
CREATE INDEX idx_spend_guard_overrides_app ON spend_guard_overrides (app_name, expires_at);
