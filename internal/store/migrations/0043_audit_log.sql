-- Audit log (docs/comparison.md's own "no audit log exists anywhere in
-- the codebase" gap): one row per request that passed requireAbility
-- (internal/api/auth.go) at more than AbilityRead, recording who did
-- what. Insert-only: no UPDATE/DELETE path exists through the app,
-- matching what an audit trail needs to mean.
CREATE TABLE audit_log (
    id          TEXT PRIMARY KEY,
    actor_type  TEXT NOT NULL,
    actor_id    TEXT NOT NULL,
    actor_name  TEXT NOT NULL,
    ability     TEXT NOT NULL,
    method      TEXT NOT NULL,
    path        TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    remote_addr TEXT NOT NULL,
    created_at  TEXT NOT NULL
);

-- Built for ListAuditEntries' own "newest first, cursor by created_at"
-- query shape, the same reasoning ListBackupHistory's index comment
-- gives for its own table.
CREATE INDEX idx_audit_log_created_at ON audit_log (created_at DESC);
