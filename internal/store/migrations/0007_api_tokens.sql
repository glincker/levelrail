-- Dashboard & auth: API tokens (TASKS.md, "Backend auth foundation").
-- The concrete near-term answer to "how does a CLI or MCP server call
-- the API non-interactively," scoped to what Phase 1 actually needs per
-- docs-local/research/theauth-go-fit-assessment.md's recommendation:
-- adopt Coolify's scoped-ability model rather than an all-or-nothing key
-- (docs-local/research/competitor-onboarding-auth-ux.md, finding 9), so
-- an MCP-issued token can be provably read-only at the token layer
-- itself, not just by convention in what a caller chooses to do with it.
--
-- Only the hash is ever stored, matching this codebase's existing
-- instinct (bcrypt for the admin password, session tokens never
-- persisted in plaintext either): the plaintext token is generated once,
-- returned once, and never recoverable after that.
CREATE TABLE api_tokens (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    -- JSON array of ability strings: "read", "read:sensitive", "write",
    -- "deploy", "root". "root" implies every other ability and is
    -- exclusive of them (enforced at the API layer, not here), the same
    -- shape Coolify's own picker uses.
    abilities TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_used_at TEXT,
    expires_at TEXT,
    revoked_at TEXT
);
