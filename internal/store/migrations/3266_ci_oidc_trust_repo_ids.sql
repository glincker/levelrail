-- Immutable GitHub repository and owner IDs a trust can bind to; empty keeps matching by name.
ALTER TABLE ci_oidc_trusts ADD COLUMN repository_id TEXT NOT NULL DEFAULT '';
ALTER TABLE ci_oidc_trusts ADD COLUMN repository_owner_id TEXT NOT NULL DEFAULT '';
