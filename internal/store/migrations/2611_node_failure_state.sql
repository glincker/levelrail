-- Live failure state of an unreachable node and the per-resource actions taken or awaiting an operator. No secrets.
CREATE TABLE node_failure_state (
    node_id        TEXT PRIMARY KEY,
    phase          TEXT NOT NULL,
    reason         TEXT NOT NULL DEFAULT '',
    unreachable_at TEXT NOT NULL,
    recovering_at  TEXT NOT NULL DEFAULT '',
    cordoned_by_us INTEGER NOT NULL DEFAULT 0,
    actions_json   TEXT NOT NULL DEFAULT '[]',
    updated_at     TEXT NOT NULL
);
