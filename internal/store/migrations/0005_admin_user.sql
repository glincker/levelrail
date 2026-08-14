-- Single admin user for TASKS.md 1.9's session auth. Phase 1 scope is
-- explicit: single admin user, session auth, no teams or RBAC yet.
-- The CHECK pins this table to exactly one row rather than a
-- real multi-row users table with no migration path to add a second row
-- later: that table is Phase 4's job (teams/RBAC), not this one's.
CREATE TABLE admin_user (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    username      TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
