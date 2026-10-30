-- Latest drift check of a stack's applied revision against live state
-- (ADR 042). One row per stack; the summary holds paths, never values.
CREATE TABLE IF NOT EXISTS stack_drift (
    stack_id     TEXT PRIMARY KEY REFERENCES stacks(id) ON DELETE CASCADE,
    status       TEXT NOT NULL DEFAULT 'none',
    revision     INTEGER NOT NULL DEFAULT 0,
    summary_json TEXT NOT NULL DEFAULT '[]',
    checked_at   TEXT NOT NULL
);
