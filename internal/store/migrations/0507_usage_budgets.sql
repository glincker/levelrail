-- Monthly budgets per project or team, checked by the budget_threshold
-- alert rule kind.
CREATE TABLE usage_budgets (
    id          TEXT PRIMARY KEY,
    scope       TEXT NOT NULL CHECK (scope IN ('project', 'team')),
    scope_id    TEXT NOT NULL,
    monthly     REAL NOT NULL CHECK (monthly > 0),
    warn_pct    REAL NOT NULL DEFAULT 80,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    UNIQUE (scope, scope_id)
);
