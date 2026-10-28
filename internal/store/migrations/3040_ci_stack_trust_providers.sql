-- Stack OIDC trusts for GitLab and CircleCI: provider, the owner-configured issuer
-- and the protected-ref rule, like app trusts. Existing rows are GitHub trusts.
CREATE TABLE ci_stack_trusts_new (
    id                 TEXT PRIMARY KEY,
    stack_id           TEXT NOT NULL REFERENCES stacks(id) ON DELETE CASCADE,
    repository         TEXT NOT NULL,
    ref_pattern        TEXT NOT NULL,
    environment        TEXT NOT NULL DEFAULT '',
    subject_pattern    TEXT NOT NULL DEFAULT '',
    verbs              TEXT NOT NULL,
    created_by         TEXT NOT NULL DEFAULT '',
    created_at         TEXT NOT NULL,
    provider           TEXT NOT NULL DEFAULT 'github',
    issuer             TEXT NOT NULL DEFAULT '',
    protected_ref_only INTEGER NOT NULL DEFAULT 0,
    UNIQUE (stack_id, provider, issuer, repository, ref_pattern, environment, subject_pattern)
);
INSERT INTO ci_stack_trusts_new (id, stack_id, repository, ref_pattern, environment, subject_pattern, verbs, created_by, created_at)
    SELECT id, stack_id, repository, ref_pattern, environment, subject_pattern, verbs, created_by, created_at FROM ci_stack_trusts;
DROP TABLE ci_stack_trusts; -- compat-break: rebuilt with every row to widen the unique key
ALTER TABLE ci_stack_trusts_new RENAME TO ci_stack_trusts; -- compat-break: completes the rebuild above
CREATE INDEX IF NOT EXISTS idx_ci_stack_trusts_repo ON ci_stack_trusts (repository, stack_id);
