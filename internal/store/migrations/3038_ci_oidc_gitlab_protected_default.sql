-- GitLab CI trusts default to protected refs only; rows made before that default
-- are moved to it, and an owner can opt out again per trust.
UPDATE ci_oidc_trusts SET protected_ref_only = 1 WHERE provider = 'gitlab';
