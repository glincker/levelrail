-- The one saved, sealed plan per account and stack. Apply runs only this plan, matched by its hash,
-- so what was reviewed is exactly what runs. The row is replaced by the next plan and removed by apply.
CREATE TABLE cloud_plans (
    account     TEXT NOT NULL,
    stack       TEXT NOT NULL,
    hash        TEXT NOT NULL,
    sealed      BLOB NOT NULL,
    created_at  TEXT NOT NULL,
    PRIMARY KEY (account, stack)
);
