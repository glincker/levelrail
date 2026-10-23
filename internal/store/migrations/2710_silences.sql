-- Silences: an operator mutes one status issue for a while or forever. Expiry is read-time.
CREATE TABLE silences (
    id         TEXT PRIMARY KEY,
    issue_key  TEXT NOT NULL,
    scope      TEXT NOT NULL DEFAULT '',
    until      TEXT,
    severity   TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX idx_silences_issue ON silences (issue_key);
