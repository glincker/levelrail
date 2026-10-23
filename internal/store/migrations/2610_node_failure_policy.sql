-- Per-node failure policy: mode (empty means the control plane default) and the unreachable grace period in seconds (0 means default). No secrets.
CREATE TABLE node_failure_policy (
    node_id       TEXT PRIMARY KEY,
    mode          TEXT NOT NULL DEFAULT '',
    grace_seconds INTEGER NOT NULL DEFAULT 0,
    updated_at    TEXT NOT NULL
);
