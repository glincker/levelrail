-- Post apply cloud state that could not be written back under the lock fence, kept sealed so the
-- real resources it describes are never orphaned; an unresolved row blocks further runs of the stack.
CREATE TABLE cloud_state_quarantine (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    account      TEXT NOT NULL,
    stack        TEXT NOT NULL,
    sealed       BLOB NOT NULL,
    fence        INTEGER NOT NULL,
    reason       TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    resolved_at  TEXT
);
CREATE INDEX cloud_state_quarantine_open_idx ON cloud_state_quarantine (account, stack, resolved_at);
