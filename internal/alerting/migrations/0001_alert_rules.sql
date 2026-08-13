-- One row per alert rule, current firing state embedded directly
-- (firing/firing_since/pending_since/last_evaluated_at/last_value)
-- rather than a separate state table: a rule and its current
-- evaluation state are always read and written together, never
-- independently, so splitting them would just mean a join on every
-- access for no real benefit, the same reasoning
-- internal/store.DesiredService's single-row shape already applies.
--
-- kind='threshold' rules use metric/comparator/threshold/
-- for_duration_seconds; kind='crashloop' rules use
-- restart_count_threshold/restart_window_seconds instead. Not split
-- into two tables: TASKS.md 2.5's own framing treats crashloop
-- detection as a built-in alert rule, not a separate system, and a
-- single polymorphic table with kind-specific columns left at their
-- zero value for the other kind mirrors how internal/store.DesiredService
-- already stores optional, kind-varying shape (Resources, Health) as
-- nullable JSON rather than separate tables per variant.
CREATE TABLE alert_rules (
    id                      TEXT PRIMARY KEY,
    name                    TEXT NOT NULL,
    kind                    TEXT NOT NULL, -- 'threshold' | 'crashloop'
    resource_id             TEXT NOT NULL, -- e.g. "service:web"

    -- threshold-kind fields, zero value for crashloop rules
    metric                  TEXT NOT NULL DEFAULT '',
    comparator              TEXT NOT NULL DEFAULT '', -- '>' | '<' | '>=' | '<='
    threshold               REAL NOT NULL DEFAULT 0,
    for_duration_seconds    INTEGER NOT NULL DEFAULT 0,

    -- crashloop-kind fields, zero value for threshold rules
    restart_count_threshold INTEGER NOT NULL DEFAULT 0,
    restart_window_seconds  INTEGER NOT NULL DEFAULT 0,

    -- notification
    notify_url              TEXT NOT NULL DEFAULT '',
    notify_kind             TEXT NOT NULL DEFAULT 'generic', -- 'generic' | 'slack' | 'discord'

    enabled                 INTEGER NOT NULL DEFAULT 1,

    -- evaluation state
    pending_since           TEXT, -- when the condition first started being satisfied, cleared once it stops or once it graduates to firing
    firing                  INTEGER NOT NULL DEFAULT 0,
    firing_since            TEXT,
    last_evaluated_at       TEXT,
    last_value              REAL,

    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL
);

CREATE INDEX idx_alert_rules_resource ON alert_rules (resource_id);
