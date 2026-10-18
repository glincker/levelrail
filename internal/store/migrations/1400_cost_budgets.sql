-- Named monthly cost budgets scoped to a project, tag or app, with the
-- level-triggered state the ticker keeps per period.
CREATE TABLE cost_budgets (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    scope        TEXT NOT NULL CHECK (scope IN ('project', 'tag', 'app')),
    scope_value  TEXT NOT NULL,
    monthly      REAL NOT NULL CHECK (monthly > 0),
    thresholds   TEXT NOT NULL DEFAULT '[50,80,100]',
    action       TEXT NOT NULL DEFAULT 'none' CHECK (action IN ('none', 'throttle', 'pause')),
    throttle_rps INTEGER NOT NULL DEFAULT 0 CHECK (throttle_rps >= 0),
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE cost_budget_state (
    budget_id     TEXT PRIMARY KEY REFERENCES cost_budgets(id) ON DELETE CASCADE,
    period        TEXT NOT NULL,
    highest       INTEGER NOT NULL DEFAULT 0,
    pct           REAL NOT NULL DEFAULT 0,
    spent         REAL NOT NULL DEFAULT 0,
    enforced      TEXT NOT NULL DEFAULT '',
    enforced_apps TEXT NOT NULL DEFAULT '[]',
    updated_at    TEXT NOT NULL
);
