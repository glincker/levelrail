-- Which CI workflows may swap their OIDC identity token for a short-lived, app-scoped deploy token.
CREATE TABLE IF NOT EXISTS ci_oidc_trusts (
    id            TEXT PRIMARY KEY,
    service_name  TEXT NOT NULL,
    repository    TEXT NOT NULL,
    ref_pattern   TEXT NOT NULL,
    environment   TEXT NOT NULL DEFAULT '',
    policy_id     TEXT NOT NULL,
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    UNIQUE (service_name, repository, ref_pattern, environment)
);
CREATE INDEX IF NOT EXISTS ci_oidc_trusts_repo ON ci_oidc_trusts (repository);
