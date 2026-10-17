CREATE TABLE app_spend_caps (
    app_name             TEXT PRIMARY KEY REFERENCES desired_services(name) ON DELETE CASCADE,
    monthly_egress_bytes INTEGER NOT NULL DEFAULT 0 CHECK (monthly_egress_bytes >= 0),
    monthly_cost_ceiling REAL NOT NULL DEFAULT 0 CHECK (monthly_cost_ceiling >= 0),
    thresholds           TEXT NOT NULL DEFAULT '[50,80,100]',
    action               TEXT NOT NULL DEFAULT 'none' CHECK (action IN ('none', 'throttle', 'pause')),
    throttle_rps         INTEGER NOT NULL DEFAULT 0 CHECK (throttle_rps >= 0),
    updated_at           TEXT NOT NULL
);

CREATE TABLE app_spend_cap_state (
    app_name     TEXT PRIMARY KEY REFERENCES desired_services(name) ON DELETE CASCADE,
    period       TEXT NOT NULL,
    enforced     TEXT NOT NULL DEFAULT '',
    nudged       TEXT NOT NULL DEFAULT '',
    highest      INTEGER NOT NULL DEFAULT 0,
    pct          REAL NOT NULL DEFAULT 0,
    egress_bytes INTEGER NOT NULL DEFAULT 0,
    cost         REAL NOT NULL DEFAULT 0,
    updated_at   TEXT NOT NULL
);

CREATE TABLE app_spend_cap_events (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    app_name TEXT NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE,
    period   TEXT NOT NULL,
    kind     TEXT NOT NULL,
    detail   TEXT NOT NULL DEFAULT '',
    pct      REAL NOT NULL DEFAULT 0,
    at       TEXT NOT NULL
);
CREATE INDEX idx_app_spend_cap_events_app_at ON app_spend_cap_events (app_name, at);
