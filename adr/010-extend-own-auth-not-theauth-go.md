# ADR 010: Extend Levelrail's own auth, don't adopt theauth-go yet

Status: Accepted

Date: 2026-08-13

## Context

The "Dashboard & auth" work pass (started 2026-08-12) needed a real answer
to a question the project plan itself already anticipates: Phase 4's
work list, item 8, names "Integration of first-party GLINCKER libraries:
theauth for the auth layer, thesvg for the icon system," with an explicit
instruction attached, "Do this here, not earlier, so the platform's own
auth is boring and proven first." theauth-go is not a hypothetical
third-party option, it's a real, first-party GLINCKER library the project
plan already earmarks as a future integration target, which is exactly why
this needed a considered decision and an ADR rather than a default
assumption either way.

Two locked decisions bear directly on the fit question. The single-binary
decision (ADR 001) commits the control plane to a single static Go binary with no separate
runtime dependencies pulled in casually. The AI-ready API design goal makes the HTTP API
contract, not the reconciler, carry the "AI-ready" promise: a future MCP
server and CLI need a way to authenticate non-interactively, and that
requirement has to be designed for now even though the MCP server itself
is Phase 4 work. Both of these point at the same open question: does
adopting theauth-go now serve Phase 1's actual auth surface ("single admin
user, session auth. No teams, no RBAC yet.", per the Phase 1 scope), or does it
front-load Phase 4 machinery into a phase that doesn't need it.

Four parallel research streams ran on 2026-08-12 to settle this and adjacent
frontend/UX questions before implementation started; the auth-specific one
produced `docs-local/research/theauth-go-fit-assessment.md`, a full
source-read of theauth-go (not README summary), covering `theauth.go`,
`config.go`, `storage.go`, `handlers.go`, `wiring.go`,
`storage/storage.go`, `storage/memory/*.go`, `docs/STABILITY.md`,
`docs/ROADMAP.md`, both bundled examples, and theauth-go's own June 2026
security/architecture/performance/reliability audit trail. TASKS.md's
"Dashboard & auth" section records the settled conclusion inline ("Don't
adopt theauth-go now... Revisit theauth-go at Phase 4") but that section is
a task queue, not the ADR the project's process requires for an architectural
decision with rejected alternatives on the record. This ADR is that
record, written after the fact for a decision that was already made and
implemented.

## Decision

Extend Levelrail's own auth, which already existed for the admin login
(`internal/api/auth.go`: bcrypt password hashing, a session store keyed by
opaque token, one `AdminUser` row in the existing SQLite store), rather
than adopting theauth-go for Phase 1. Concretely, this pass built:

- **Scoped API tokens with an abilities model**
  (`internal/store/tokens.go` + migration 0007,
  `api_tokens(id, name, token_hash, abilities, created_at, last_used_at,
  expires_at, revoked_at)`). Only a SHA-256 hash of the token is ever
  stored, generated with the existing `randomToken` helper. Abilities are
  `read`, `read:sensitive`, `write`, `deploy`, `root` (root exclusive of
  the rest, checked fresh per request via `hasAbility` in
  `internal/api/abilities.go`, never a cached snapshot), matching
  Coolify's scoped-ability model rather than Dokploy's all-or-nothing key,
  specifically so an MCP-issued token can be provably read-only at the
  token layer, per the AI-ready API design goal. `requireAbility` middleware accepts
  either a session (implicitly root, since Phase 1 has exactly one human
  identity) or a bearer token scoped to the route's required ability;
  token-management routes stay session-only on purpose, so a token can
  never mint or revoke another token on its own behalf.
- **First-run registration** (`POST /api/v1/auth/register`), coexisting
  with the existing env-var bootstrap, using a plain `INSERT`
  (`store.CreateAdminUser`) whose correctness rests on `admin_user`'s own
  `PRIMARY KEY (id = 1)` constraint rather than a caller-side
  check-then-act, so concurrent registration attempts can't both win.
- **Login rate limiting** (`internal/api/ratelimit.go`'s `loginLimiter`),
  per-(client IP, attempted username), exponential backoff after a grace
  period, gating before any bcrypt comparison runs so probing a username
  costs the same as guessing its password. Session TTL is configurable
  (`APP_SESSION_TTL`) rather than hardcoded, per the project's
  no-hardcoded-thresholds rule.
- **A locked-out recovery path**: `levelrail recover-admin`, a subcommand
  of the same binary (per the single-binary decision, ADR 001, not a second binary),
  container-native (`docker exec <container> levelrail recover-admin`),
  writing a bcrypt hash directly via `store.UpsertAdminUser`.
- **Dev mode**: `APP_DEV_MODE=1`, build-tag gated
  (`devmode_debug.go`/`devmode_release.go`, mirroring the existing
  `embedweb` pattern), which bootstraps a fixed `dev`/`dev` admin through
  the same idempotent `BootstrapAdmin` path already used for env-var
  bootstrap, so the real login and auth-guard code stays exercised rather
  than bypassed.

All of this is additive to the existing bcrypt+session-cookie design, uses
the SQLite store the project is already committed to (ADR 007), and
introduces zero new dependencies.

## Rejected alternatives

- **Adopt theauth-go now**, wiring it in for Phase 1's admin login and the
  near-term CLI/API-token need. Rejected on three independent grounds, any
  one of which would be sufficient on its own:
  1. **Storage mismatch is a regression, not a neutral trade.**
     theauth-go ships three storage backends: Postgres ("production
     default"), MySQL, and Memory, explicitly documented in
     `storage/memory/doc.go` as "intended for tests, examples, and local
     development... offers no persistence guarantees. Production
     deployments should use storage/postgres." There is no SQLite
     adapter. The embedded-SQLite decision (ADR 007) locks the control plane to embedded SQLite
     via `modernc.org/sqlite`, with Postgres explicitly deferred "only if
     HA control plane becomes a real request." Levelrail's current auth
     already accepts that a restart wipes sessions, but the admin account
     itself survives restarts today because it lives in the real SQLite
     store. Swapping in theauth-go's Memory backend would make the admin
     account itself evaporate on every restart, not just sessions, a
     strict regression from what already works.
  2. **The `Storage` interface is one flat, all-or-nothing contract.**
     `storage.go` defines a base `Storage` interface at 364 lines, and per
     theauth-go's own `docs/STABILITY.md`, "the base `Storage` interface
     only grows in a v2.0 release," meaning any adapter, including a
     hypothetical SQLite one, has to implement OAuth accounts, WebAuthn
     credentials, TOTP secrets, recovery codes, and multi-tenancy tables
     whether or not a deployment uses any of them. The existing Postgres
     adapter that does implement the full interface is 4,537 lines. Phase
     1's actual auth requirement, per the project's phased scope, is "hash a password,
     create a session, look up a session." Building and maintaining a
     from-scratch SQLite adapter at that scale to serve that requirement
     is disproportionate by any reasonable measure.
  3. **Real scope overshoot past "single admin, no teams yet."** Even
     theauth-go's smallest possible config (`Storage: memory.New(),
     BaseURL: "..."`) unconditionally wires magic-link sign-in,
     email+password sign-in/signup/reset, and rate-limited session cookie
     issuance through `Mount`. Email+password signup creates arbitrary new
     users, which doesn't fit a single-admin model without extra guard
     code layered on top to reject anything but the one admin account.
     None of WebAuthn, TOTP, SAML, SCIM, RBAC, the OAuth 2.1 authorization
     server, or agent identities have to be turned on, which is the good
     part of theauth-go's opt-in `Config` design, but even with nothing
     else enabled, Phase 1 would be carrying a multi-user auth system for
     a product whose Phase 1 scope is explicitly one admin
     user. The one integration path that would justify adopting now for a
     real Phase 1 need, agent/CLI bearer tokens, requires
     `Config.AuthorizationServer` plus `Config.AgentIdentity`, which pulls
     in a second large interface (`OAuthServerStorage`: OAuth clients,
     authorization codes, refresh tokens, JWKS keys, agents, agent
     credentials, delegation grants) layered on top of the base `Storage`
     interface, again requiring durable storage with no SQLite adapter to
     provide it.

  A secondary, values-level friction point: `handlers.go`'s `Mount`
  signature takes a `chi.Router`, not `http.Handler`. Levelrail's own
  `internal/api/router.go` deliberately chose stdlib `net/http`'s
  pattern-based mux over a router framework like Chi, documented in that
  file's own package comment. This alone would not have blocked adoption
  (theauth-go's own `examples/stdlib-app/main.go` shows wrapping just the
  `/auth` subtree under Chi rather than converting the whole router), but
  it's a real friction point on top of the storage and scope problems
  above, not a reason by itself.

- **Do nothing until Phase 4, ship Phase 1 without API tokens at all.**
  Rejected because the AI-ready API design goal requires the HTTP API contract to be
  designed for the future MCP layer starting now, and the concrete
  near-term need (a CLI or early MCP prototype getting a bearer token
  without a human clicking through a login form) is real in this pass, not
  hypothetical. Deferring it to Phase 4 would mean either retrofitting
  token auth onto an API surface already built without it, or blocking
  early CLI/automation work that doesn't need to wait.

## Consequences

- Phase 1's auth surface stays small, uses only the SQLite store the
  project already depends on (ADR 007), and adds zero new dependencies:
  no Chi, no OAuth 2.1 authorization server, no second storage interface
  to satisfy.
- The API-token abilities model (`read`, `read:sensitive`, `write`,
  `deploy`, `root`) is Levelrail's own design, not theauth-go's
  OAuth-based agent-identity model (`client_credentials`, delegation
  grants, actor chains). If theauth-go is adopted at Phase 4, this is a
  real reconciliation cost, not a free swap: theauth-go's agent-identity
  surface (`CreateAgent`, `MintAgentCredential`, `GrantDelegation`,
  `ExchangeToken` for RFC 8693 token exchange) is a materially different
  and more capable model than a flat abilities table, and migrating
  existing tokens, callers, and any MCP prototype built against the
  current scheme onto it will be real work, not a config change.
- The research doc is explicit that this is not a permanent no: theauth-go's
  MCP/agent-authorization story is "the one part of this evaluation that
  is a legitimately strong, forward-looking fit" for the platform's AI-ready
  API goal, specifically because `mcpresource` is a separately-importable,
  zero-dependency module that answers "how does a future MCP server
  validate a caller's identity" with RFC 9728 protected-resource metadata
  and RFC 8693 actor-chain walking, not a bespoke scheme. That fit is real
  at Phase 4, when teams, RBAC, and the MCP server itself are actually in
  scope per the project's phased roadmap, not before.
- Revisit this decision at Phase 4. At that point Levelrail's needs
  (multi-user, scoped agent delegation, possibly SAML/SCIM for an
  enterprise self-hosted tier) would actually exercise the parts of
  theauth-go that sit unused today, changing the cost/benefit calculus
  this ADR is based on. The research doc also names a concrete way to
  remove the single biggest blocker before that pass starts: since
  GLINCKER owns both projects, a SQLite storage adapter for theauth-go
  itself could be built between now and Phase 4, using the existing
  Postgres/MySQL adapters as the template, so the "no SQLite adapter"
  blocker in this ADR's rejected alternatives doesn't have to still be
  true when the decision is revisited.
- If theauth-go is adopted at Phase 4, the migration is not incremental:
  per the research doc's own sketch, `internal/api/auth.go`'s
  `sessionStore`, `handleLogin`, `handleLogout`, `requireAuth`, and
  `BootstrapAdmin` would all be deleted and replaced by a `theauth.TheAuth`
  instance, `router.go`'s `Handler()` would need a Chi subtree wrapper or
  a full router migration, and a new SQLite adapter satisfying the full
  `Storage` interface (and `OAuthServerStorage`, if agent auth is wanted)
  would need to be built and validated against theauth-go's `storagetest`
  contract suite, plausibly the largest single piece of work in the whole
  Phase 1/2 auth effort. Building custom auth now does not eliminate that
  future cost; it defers it to the phase where the capability is actually
  needed, which is the trade this ADR makes deliberately.
