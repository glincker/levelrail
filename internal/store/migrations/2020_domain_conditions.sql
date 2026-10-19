-- Per-domain problems the ingress controller found on its last pass (dropped
-- or downgraded domains), so the domain list can say why instead of staying silent.
CREATE TABLE domain_conditions_state (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    conditions TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
