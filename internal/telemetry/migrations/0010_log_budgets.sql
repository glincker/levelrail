-- Per-app log volume budgets. max_bytes is the stored-log ceiling for one
-- resource; the retention sweep trims that app's oldest rows past it.
CREATE TABLE log_budgets (
    resource_id TEXT PRIMARY KEY,
    max_bytes   INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

-- Running total of what budget enforcement has dropped per resource.
CREATE TABLE log_budget_drops (
    resource_id TEXT PRIMARY KEY,
    lines       INTEGER NOT NULL,
    bytes       INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);
