-- Deploy guardrail rule sets per scope (org, project, environment). rules is
-- a JSON object of rule id to {mode, values, limit}; an absent row means no
-- rules at that scope.
CREATE TABLE deploy_policies (
    scope      TEXT NOT NULL,
    scope_id   TEXT NOT NULL,
    rules      TEXT NOT NULL,
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    PRIMARY KEY (scope, scope_id)
);
