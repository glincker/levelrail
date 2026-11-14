-- Opt-in git sync of a stack: the source it plans from and the latest plan result.
-- Rows hold a credential name and plan counts only, never repository content or secrets.
CREATE TABLE IF NOT EXISTS stack_sync (
    stack_id         TEXT PRIMARY KEY REFERENCES stacks(id) ON DELETE CASCADE,
    repo_url         TEXT NOT NULL,
    branch           TEXT NOT NULL,
    path             TEXT NOT NULL DEFAULT '',
    credential_ref   TEXT NOT NULL DEFAULT '',
    interval_seconds INTEGER NOT NULL DEFAULT 0,
    webhook_only     INTEGER NOT NULL DEFAULT 0,
    enabled          INTEGER NOT NULL DEFAULT 0,
    last_sha         TEXT NOT NULL DEFAULT '',
    notified_sha     TEXT NOT NULL DEFAULT '',
    plan_hash        TEXT NOT NULL DEFAULT '',
    counts_json      TEXT NOT NULL DEFAULT '{}',
    pending          INTEGER NOT NULL DEFAULT 0,
    code             TEXT NOT NULL DEFAULT '',
    checked_at       TEXT NOT NULL DEFAULT '',
    updated_at       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_stack_sync_enabled ON stack_sync (enabled);
