-- OIDC trusts for stacks: which CI identities may exchange a provider token for
-- a short-lived, stack-scoped token, and with which verbs (ADR 042).
CREATE TABLE IF NOT EXISTS ci_stack_trusts (
    id              TEXT PRIMARY KEY,
    stack_id        TEXT NOT NULL REFERENCES stacks(id) ON DELETE CASCADE,
    repository      TEXT NOT NULL,
    ref_pattern     TEXT NOT NULL,
    environment     TEXT NOT NULL DEFAULT '',
    subject_pattern TEXT NOT NULL DEFAULT '',
    verbs           TEXT NOT NULL,
    created_by      TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    UNIQUE (stack_id, repository, ref_pattern, environment, subject_pattern)
);

CREATE INDEX IF NOT EXISTS idx_ci_stack_trusts_repo ON ci_stack_trusts (repository, stack_id);
