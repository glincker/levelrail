-- CI OIDC trusts for GitLab and CircleCI: which provider, the owner-configured
-- issuer whose keys verify the token, and an optional protected-ref rule.
-- Existing rows are GitHub trusts.
ALTER TABLE ci_oidc_trusts ADD COLUMN provider TEXT NOT NULL DEFAULT 'github';
ALTER TABLE ci_oidc_trusts ADD COLUMN issuer TEXT NOT NULL DEFAULT '';
ALTER TABLE ci_oidc_trusts ADD COLUMN protected_ref_only INTEGER NOT NULL DEFAULT 0;
