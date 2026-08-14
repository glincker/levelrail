-- Phase 3 (TASKS.md 3.1): the node registry. Nothing before this
-- migration has any concept of more than one node at all; every
-- controller and every store table implicitly assumes the single local
-- Docker daemon this process itself talks to.
--
-- A node row is created by the join-token enrollment flow (ADR 003:
-- "the control plane issues a one-time join token, agent exchanges it
-- for a client certificate"), not by an operator directly POSTing node
-- details: there is deliberately no INSERT path here from
-- internal/api's 3.1 routes, only list/get/delete. Node creation itself
-- is TASKS.md 3.2 scope (the real gRPC agent binary calling the
-- exchange), once there's an actual agent to issue a certificate to.
CREATE TABLE nodes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    address TEXT NOT NULL DEFAULT '',
    -- pending (enrolled, never yet connected) | online | offline | cordoned.
    -- Enforced at the Go layer (store.NodeStatus), not a CHECK constraint,
    -- matching how alert_rules.kind/comparator and desired_services'
    -- equivalent string columns are validated elsewhere in this codebase.
    status TEXT NOT NULL DEFAULT 'pending',
    cert_fingerprint TEXT NOT NULL DEFAULT '',
    joined_at TEXT,
    last_seen_at TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- One-time join tokens (ADR 003), the same "only the hash is ever
-- stored" shape 0007_api_tokens.sql already established: the plaintext
-- token is generated once, shown to the operator once, and never
-- recoverable from this table again.
CREATE TABLE node_join_tokens (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    expires_at TEXT NOT NULL,
    -- NULL until exchanged for a node enrollment; single-use is enforced
    -- by an atomic conditional UPDATE at the Go layer
    -- (MarkNodeJoinTokenUsed), not by this schema alone, the same
    -- concurrency-safety lesson CreateAdminUser's own doc comment
    -- already documents for a different single-use resource.
    used_at TEXT
);
