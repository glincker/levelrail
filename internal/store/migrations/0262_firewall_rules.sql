-- Declarative host firewall rules an operator asks
-- internal/reconcile/firewall to converge onto the local host's ufw.
-- v1 scope: node_id is always '' (the control plane's own node, the
-- same sentinel DesiredService.NodeID/DesiredDatabase.NodeID already
-- use); a non-local node_id is accepted but skipped by the reconciler
-- until a per-node agent RPC exists.
CREATE TABLE firewall_rules (
    id          TEXT PRIMARY KEY,
    node_id     TEXT NOT NULL DEFAULT '',
    port        INTEGER NOT NULL,
    protocol    TEXT NOT NULL DEFAULT 'tcp',
    source_cidr TEXT NOT NULL DEFAULT '',
    action      TEXT NOT NULL DEFAULT 'allow',
    label       TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL
);

CREATE INDEX idx_firewall_rules_node_id ON firewall_rules(node_id);
