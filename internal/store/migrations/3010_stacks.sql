-- Stacks: a named set of resources applied from git as numbered revisions,
-- with an ownership ledger and a per-stack apply lease (ADR 042).
CREATE TABLE IF NOT EXISTS stacks (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL UNIQUE,
    project_id        TEXT NOT NULL,
    environment_id    TEXT NOT NULL DEFAULT '',
    allow_prune       INTEGER NOT NULL DEFAULT 0,
    drift_policy      TEXT NOT NULL DEFAULT 'warn',
    pinned_revision   INTEGER NOT NULL DEFAULT 0,
    require_approval  INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL,
    created_by        TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS stack_revisions (
    stack_id          TEXT NOT NULL REFERENCES stacks(id) ON DELETE CASCADE,
    number            INTEGER NOT NULL,
    status            TEXT NOT NULL,
    plan_hash         TEXT NOT NULL DEFAULT '',
    plan_json         BLOB,
    rendered_docs_gz  BLOB,
    rollback_of       INTEGER NOT NULL DEFAULT 0,
    idempotency_key   TEXT NOT NULL DEFAULT '',
    actor             TEXT NOT NULL DEFAULT '',
    token_id          TEXT NOT NULL DEFAULT '',
    git_repo          TEXT NOT NULL DEFAULT '',
    git_ref           TEXT NOT NULL DEFAULT '',
    git_sha           TEXT NOT NULL DEFAULT '',
    ci_run_url        TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL,
    finished_at       TEXT,
    result_json       BLOB,
    PRIMARY KEY (stack_id, number)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_stack_revisions_idem
    ON stack_revisions (stack_id, idempotency_key) WHERE idempotency_key != '';

-- The ownership ledger: a resource has at most one owning stack.
CREATE TABLE IF NOT EXISTS stack_resources (
    stack_id        TEXT NOT NULL REFERENCES stacks(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL,
    key             TEXT NOT NULL,
    live_id         TEXT NOT NULL DEFAULT '',
    since_revision  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (stack_id, kind, key),
    UNIQUE (kind, key)
);

CREATE TABLE IF NOT EXISTS stack_locks (
    stack_id    TEXT PRIMARY KEY REFERENCES stacks(id) ON DELETE CASCADE,
    holder      TEXT NOT NULL,
    revision    INTEGER NOT NULL DEFAULT 0,
    expires_at  TEXT NOT NULL
);
