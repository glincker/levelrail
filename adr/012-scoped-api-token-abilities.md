# ADR 012: Scoped API token abilities, not an all-or-nothing key

Status: Accepted

Date: 2026-08-12

## Context

CLAUDE.md 1 states the positioning directly: "a lightweight, AI-ready
Coolify alternative," and defines what that phrase is allowed to mean:
"'AI-ready' means the HTTP API is designed from Phase 1 so the Phase 4 MCP
layer bolts on cleanly (see 4.11): it does not mean AI enters the
reconciliation path." Section 4.11 restates the same boundary from the
API side: the MCP server is "a thin wrapper once the HTTP API is well
designed, which is the point," built later, but the API contract has to
be designed now assuming it will exist.

The founder requirement behind this ADR is broader than MCP alone:
third-party tools, the Phase 4 CLI (`levelrail deploy`, `logs`, `exec`,
`rollback`), and AI agents should all be able to control the platform
through the same token-scoped HTTP API that a human uses through the
session-cookie login, not a second, separate integration surface. That
requirement only holds up if a token's blast radius is provably bounded.
A credential handed to a CI pipeline, a diagnostic MCP tool, or a
third-party integration needs a ceiling on what it can do that isn't just
"whatever the maintainer promises the code will do with it." Single-scope,
all-or-nothing API keys cannot express that ceiling: the token itself
carries no information about what it should be trusted to do, only who
issued it.

This decision was researched in
`docs-local/research/competitor-onboarding-auth-ux.md`, specifically
"finding 9" in its synthesis section, which compares Coolify's and
Dokploy's token models on exactly this axis. `TASKS.md`'s "Dashboard &
auth" section records the resulting settled decision ("API tokens use
Coolify's scoped-ability model... This is what lets an MCP-issued token
be provably read-only at the token layer, per CLAUDE.md 4.11") and its
"API tokens (2026-08-12)" checklist item records what was actually built:
`internal/store/tokens.go`, migration 0007 (`api_tokens(id, name,
token_hash, abilities, created_at, last_used_at, expires_at,
revoked_at)`), and `internal/api/abilities.go`.

## Decision

API tokens carry one or more of six abilities: `read`, `read:sensitive`,
`write`, `write:sensitive`, `deploy`, `root`. This ADR was originally
drafted describing a five-ability set that omitted `write:sensitive`;
that was a real gap against Coolify's own model (finding 9's source
material lists all six), caught by re-reading `internal/api/router.go`
while writing this ADR: `PUT /api/v1/apps/{name}/secrets/{key}` was
gated behind plain `AbilityWrite`, meaning a token scoped for ordinary
app-config edits could also overwrite encrypted secret values. Fixed in
the same pass this ADR was written in: `AbilityWriteSensitive` was added
to `internal/api/abilities.go`, and the secrets route now requires it
specifically, TDD'd (`TestHandleSetSecret_PlainWriteTokenForbidden`,
`TestHandleSetSecret_WriteSensitiveTokenSucceeds`) and live-verified
against a real binary (a plain-`write` bearer token gets 403 on the
secrets route, a `write:sensitive` token reaches the handler).

- `root` implies every other ability and is mutually exclusive of them at
  mint time: `validateAbilities` in `internal/api/abilities.go` rejects a
  request that combines `root` with any other ability string, matching
  Coolify's own rule (its `ApiTokens.php` clears every other checkbox the
  moment `root` is selected: "`root` is exclusive, not additive," per the
  research doc).
- Abilities are checked fresh on every request via `hasAbility`, never
  cached or snapshotted onto a session or request object at token-mint
  time. This mirrors Coolify's own explicit reasoning, quoted directly in
  the research doc from `ApiTokens.php:67,115-117`: "Re-evaluate policies
  fresh, never trust stored snapshot booleans... can be replayed from
  another user's session." A revoked or expired token, or one whose
  abilities have changed, is denied on the very next request, not on the
  next time some cached check happens to run.
- A new `requireAbility` middleware gates routes. It accepts either a
  session (implicitly `root`, because Phase 1 has exactly one human
  identity, the single admin user) or a bearer token that carries the
  specific ability the route requires. Apps, deploys, and secrets routes
  use `requireAbility` with the appropriate ability instead of the
  previous plain `requireAuth`, so a `read`-scoped token cannot reach a
  deploy-triggering route and a `deploy`-scoped token cannot read secret
  values gated behind `read:sensitive`.
- Token management itself, minting (`POST /api/v1/auth/tokens`), listing
  (`GET /api/v1/auth/tokens`), and revoking (`DELETE
  /api/v1/auth/tokens/{id}`), stays `requireAuth`-only: session-only,
  deliberately not reachable by any bearer token regardless of its
  abilities, including `root`-scoped ones. This closes an obvious
  self-escalation path before it can exist: a token can never mint a new
  token or revoke another token on its own behalf. Only a logged-in human
  session can manage the token inventory.
- `token_hash` stored in `internal/store/tokens.go` is the SHA-256 hex of
  a `crypto/rand`-generated token (`randomToken`, reused from the
  existing `auth.go`); only the hash is ever persisted, the plaintext is
  returned exactly once at mint time.

## Rejected alternatives

- **Dokploy's all-or-nothing API key.** The research doc documents this
  concretely: Dokploy's `apiKey` plugin (`better-auth`, configured in
  `packages/server/src/lib/auth.ts:423-426`) has "no scope/ability picker
  at all, no read-only-vs-write distinction, no per-project restriction."
  A generated key "authenticates as the creating user with that user's
  full role," resolved in `validateRequest()` (`auth.ts:492-524`), which
  "mints a session-equivalent object with the user's real `role`." The
  research doc's own framing of why this fails Levelrail's use case
  specifically: "Dokploy's key system is rich on lifecycle knobs (rate
  limits, expiry, prefixing) but has no authorization granularity at all:
  a key is either 'this whole account' or nothing." For a platform whose
  own CLAUDE.md 4.11 commits to an MCP server and a Phase 4 CLI sitting
  on the same API, this model means a leaked or misbehaving AI-issued
  token would have the same reach as a stolen human session: full
  platform control, not a bounded capability. That is precisely the
  outcome the "AI-ready... does not mean AI enters the reconciliation
  path" boundary in CLAUDE.md 1 and 4.11 is trying to prevent at the API
  layer, and an all-or-nothing key cannot express a boundary at all.
- **A more granular per-resource ACL system** (e.g. per-app permissions,
  per-environment scoping). Rejected as over-engineered for the current
  phase, not as wrong in principle. CLAUDE.md 6's Phase 1 exit criteria
  are explicit: "Single admin user, session auth. No teams, no RBAC yet,"
  and Phase 4 is where "Teams, projects, environments, RBAC with a small
  role set" actually lands, with an explicit warning attached even there
  to "resist the urge to build a permission matrix." Building per-app
  ACLs now would mean designing and maintaining resource-scoped
  authorization for a single-admin, single-tenant instance where every
  resource already belongs to the one identity that exists. Coolify's
  five-ability, resource-agnostic model is the right altitude for this
  phase: coarse enough to build and reason about now, expressive enough
  to make an MCP tool's token provably bounded, deferred to whenever
  teams/RBAC actually land in Phase 4.

## Consequences

- This is the mechanism, not just the policy, behind CLAUDE.md 4.11's
  "AI-ready" claim. Once the Phase 4 MCP server exists, a diagnostic tool
  (list apps, get logs, get metrics, explain a failed build) can be
  issued a token that is provably `read`-scoped at the token layer, and a
  "trigger deploy" tool can be issued a token that has `deploy` without
  `write` on arbitrary resources. This is true by construction (checked
  fresh via `hasAbility` on every request) rather than by convention in
  what the MCP server chooses to call, which is exactly the distinction
  the research doc's finding 9 draws between Coolify's and Dokploy's
  models.
- The ability list is closed and hardcoded right now: `validateAbilities`
  rejects any ability string outside the six named above. Adding a new,
  more granular ability later (for example, a scope narrower than
  `write`, or a per-resource variant once Phase 4's RBAC lands) is a
  breaking-ish change to the token model, not an additive one: existing
  tokens will not automatically gain a new ability just because it now
  exists, and any route that starts requiring a new ability needs a
  migration story for tokens minted under the old five-ability set.
  Worth flagging explicitly for whoever extends this at Phase 4, so the
  RBAC work doesn't silently assume today's tokens already carry
  tomorrow's scopes.
- Because token management routes are session-only, there is currently no
  way for a token holder, however high its abilities, to provision
  another token programmatically. Any future workflow that wants a
  service to mint its own short-lived tokens (a CI system, for example)
  will need a deliberate, separately-designed exception to this rule
  rather than an incidental one, since the whole point of the current
  design is that no such path exists yet.
- 26 tests cover this behavior today, including revoked, expired,
  wrong-ability, and root-implies-everything cases, at 77.4% coverage on
  `internal/api` and 78.4% on `internal/store`. Any change to
  `validateAbilities`, `hasAbility`, or `requireAbility` should extend
  that suite rather than treat it as settled, since this is exactly the
  kind of trust-boundary code CLAUDE.md 8 already singles out for
  single-session, human-reviewed work rather than parallel agents.
