-- Cost showback: operator pricing, per-node monthly bills, and a daily
-- snapshot so cost history outlives telemetry retention.
CREATE TABLE usage_cost_settings (
    id                 INTEGER PRIMARY KEY CHECK (id = 1),
    mode               TEXT NOT NULL DEFAULT 'unit',
    currency           TEXT NOT NULL DEFAULT 'USD',
    cpu_core_hour      REAL NOT NULL DEFAULT 0,
    mem_gib_hour       REAL NOT NULL DEFAULT 0,
    storage_gib_month  REAL NOT NULL DEFAULT 0,
    egress_gib         REAL NOT NULL DEFAULT 0,
    idle_spread_pct    REAL NOT NULL DEFAULT 0,
    basis              TEXT NOT NULL DEFAULT 'avg',
    cpu_weight_pct     REAL NOT NULL DEFAULT 50,
    updated_at         TEXT NOT NULL
);

CREATE TABLE usage_cost_nodes (
    node_id        TEXT PRIMARY KEY,
    monthly_price  REAL NOT NULL DEFAULT 0,
    vcpus          REAL NOT NULL DEFAULT 0,
    memory_gib     REAL NOT NULL DEFAULT 0,
    updated_at     TEXT NOT NULL
);

CREATE TABLE usage_cost_daily (
    day             TEXT NOT NULL,
    kind            TEXT NOT NULL,
    name            TEXT NOT NULL,
    team_id         TEXT NOT NULL DEFAULT '',
    project_id      TEXT NOT NULL DEFAULT '',
    environment_id  TEXT NOT NULL DEFAULT '',
    node_id         TEXT NOT NULL DEFAULT '',
    up_seconds      REAL NOT NULL DEFAULT 0,
    cpu_avg_cores   REAL NOT NULL DEFAULT 0,
    cpu_p50_cores   REAL NOT NULL DEFAULT 0,
    mem_avg_bytes   REAL NOT NULL DEFAULT 0,
    mem_p50_bytes   REAL NOT NULL DEFAULT 0,
    storage_bytes   REAL NOT NULL DEFAULT 0,
    egress_bytes    REAL NOT NULL DEFAULT 0,
    compute_cost    REAL NOT NULL DEFAULT 0,
    storage_cost    REAL NOT NULL DEFAULT 0,
    egress_cost     REAL NOT NULL DEFAULT 0,
    idle_cost       REAL NOT NULL DEFAULT 0,
    PRIMARY KEY (day, kind, name)
);

CREATE INDEX usage_cost_daily_project ON usage_cost_daily (project_id, day);
