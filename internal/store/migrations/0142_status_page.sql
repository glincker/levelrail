-- Opt-in public status page: one settings row, operator-chosen components
-- and per-day probe counters that back the 90-day uptime bars.
CREATE TABLE status_page_settings (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    enabled       INTEGER NOT NULL DEFAULT 0,
    title         TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    custom_domain TEXT NOT NULL DEFAULT '',
    updated_at    TEXT NOT NULL
);

-- display_name is the only name ever shown publicly; target (an app name,
-- a domain or a check URL) never leaves the control plane.
CREATE TABLE status_components (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL,   -- app | domain | check
    target       TEXT NOT NULL,
    display_name TEXT NOT NULL,
    position     INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE status_component_daily (
    component_id     TEXT NOT NULL REFERENCES status_components(id) ON DELETE CASCADE,
    day              TEXT NOT NULL,   -- YYYY-MM-DD, UTC
    ok_samples       INTEGER NOT NULL DEFAULT 0,
    degraded_samples INTEGER NOT NULL DEFAULT 0,
    down_samples     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (component_id, day)
);
