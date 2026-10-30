-- CI OIDC trusts are unique per provider and issuer as well, so a GitHub and a GitLab
-- project with the same path, or two GitLab instances, can each be trusted.
CREATE TABLE ci_oidc_trusts_new (
    id                 TEXT PRIMARY KEY,
    service_name       TEXT NOT NULL,
    repository         TEXT NOT NULL,
    ref_pattern        TEXT NOT NULL,
    environment        TEXT NOT NULL DEFAULT '',
    policy_id          TEXT NOT NULL,
    created_by         TEXT NOT NULL DEFAULT '',
    created_at         TEXT NOT NULL,
    provider           TEXT NOT NULL DEFAULT 'github',
    issuer             TEXT NOT NULL DEFAULT '',
    protected_ref_only INTEGER NOT NULL DEFAULT 0,
    UNIQUE (service_name, provider, issuer, repository, ref_pattern, environment)
);
INSERT INTO ci_oidc_trusts_new (id, service_name, repository, ref_pattern, environment, policy_id, created_by, created_at, provider, issuer, protected_ref_only)
    SELECT id, service_name, repository, ref_pattern, environment, policy_id, created_by, created_at, provider, issuer, protected_ref_only FROM ci_oidc_trusts;
DROP TABLE ci_oidc_trusts; -- compat-break: rebuilt with every row and column to widen the unique key
ALTER TABLE ci_oidc_trusts_new RENAME TO ci_oidc_trusts; -- compat-break: completes the rebuild above
CREATE INDEX IF NOT EXISTS ci_oidc_trusts_repo ON ci_oidc_trusts (repository);
