# ADR 013: Backend-side, build-tag-gated dev-mode admin bootstrap

Status: Accepted

Date: 2026-08-13

## Context

The "Dashboard & auth" work pass (TASKS.md, started 2026-08-12) surfaced a
local-iteration problem distinct from the auth features themselves: once
first-run registration, rate limiting, and the login screen existed, every
`npm run dev` reload or Vite HMR full refresh, and every restart of the Go
binary while iterating on the backend, lost the session. This is not a bug,
it is the documented behavior of `sessionStore`, whose own doc comment says
a restart invalidates every session, because the store is in-memory. A
founder iterating on dashboard UI locally would otherwise have to re-run
the login form dozens of times per session.

A companion gap made this worse in practice: `web/vite.config.ts` had no
`server.proxy` entry for `/api` at all, so `npm run dev`'s frontend could
not reach the Go backend by default regardless of auth. Dev mode only
matters once that proxy exists, so the same task added a standard Vite
`server.proxy` entry (`/api` to `http://localhost:8080`, `changeOrigin:
true`), dev-server-only, never read by `vite build`.

The design work for the auth bypass itself is recorded in
`docs-local/research/dashboard-gap-audit-and-devmode.md`, Part 2 ("Dev-mode
design sketch, not implemented"). That doc is a sketch, not a spec, and the
shipped implementation is simpler than it in one respect worth recording
here, not just in TASKS.md.

## Decision

`APP_DEV_MODE=1` bootstraps a fixed, well-known `dev`/`dev` admin account
on startup, gated by a compile-time build-tag pair in `internal/api` that
deliberately mirrors an existing pattern in this codebase rather than
inventing a new one: `web/embed_real.go` (`//go:build embedweb`) versus
`web/embed_stub.go` (`//go:build !embedweb`), the mechanism that decides
whether `web.DistFS` actually embeds the built frontend. Dev mode reuses
the same `embedweb` tag with the polarity reversed:

- `internal/api/devmode_debug.go` (`//go:build !embedweb`) defines
  `devModeEnabled() bool` by reading `os.Getenv("APP_DEV_MODE") == "1"`.
  This is the one deliberate exception to `NewRouter`'s convention that
  `internal/api` never reads the environment directly; the exception
  exists specifically to gate the dev-mode bypass itself.
- `internal/api/devmode_release.go` (`//go:build embedweb`) defines the
  same function to unconditionally `return false`, without reading
  `APP_DEV_MODE` at all. A real `-tags embedweb` release build never even
  looks at the env var.

`internal/api/devmode.go` holds the rest: `maybeBootstrapDevAdmin(ctx, s,
logger, enabled bool)` is the testable core (a pure no-op when `enabled` is
false), and `MaybeBootstrapDevAdmin(ctx, s, logger)` is the real entry
point, supplying `devModeEnabled()` as the `enabled` argument.
`cmd/levelrail/main.go`'s `run()` calls it right after the existing
`bootstrapAdmin(ctx, db)` call, not before: `BootstrapAdmin` no-ops once
any admin account exists, so calling dev-mode bootstrap second means an
operator-set `APP_ADMIN_USERNAME`/`APP_ADMIN_PASSWORD` always wins over the
fixed dev fallback regardless of call order, but the ordering documents
that intent directly in `main.go`. When enabled, `maybeBootstrapDevAdmin`
logs a `slog.Warn` on every startup naming the fixed username and password,
then delegates to `BootstrapAdmin(ctx, s, devModeUsername, devModePassword)`,
the exact same function the real env-var-driven admin bootstrap already
uses. `BootstrapAdmin` is itself idempotent, a no-op once any admin exists,
so `MaybeBootstrapDevAdmin` only ever creates the `dev`/`dev` account on a
genuinely empty data directory.

Critically, `requireAuth` (`internal/api/auth.go`) and the real
`handleLogin` HTTP path are completely untouched by this change. There is
no second "skip auth" code branch anywhere in the router or middleware.
The frontend still has to submit real login credentials through the real
`POST /api/v1/auth/login` endpoint; they are just trivially known ones.

This is a simpler mechanism than the design sketch in
`dashboard-gap-audit-and-devmode.md` considered. The sketch drew a
parallel to `internal/api/testutil_test.go`'s `loginTestSession` helper,
which bootstraps an admin and then drives the real `handleLogin` path to
mint a real session cookie, and floated dev mode doing the same at startup
so a session would already exist rather than only an account. The shipped
version does not fabricate a session: it stops at ensuring the account
exists. That is sufficient because the actual friction being solved is
typing real credentials repeatedly, not the login request itself, an
already-known `dev`/`dev` pair is a trivial thing to submit through the
real form; and because `BootstrapAdmin`'s existing idempotency already does
the necessary work of making dev mode safe to leave enabled across restarts
without recreating or clobbering the account, there was no remaining
problem that a fabricated session would have solved.

## Rejected alternatives

- **A frontend-side bypass gated on `import.meta.env.DEV`**, skipping the
  auth guard in the React router entirely during Vite dev-server mode.
  Rejected because it means the exact code being iterated on, the app
  shell's auth-aware routing in `web/src/routes/__root.tsx`, never
  actually gets exercised in dev mode. That defeats the purpose of a
  dashboard dev loop: the founder would be iterating on a UI whose
  auth-guard code path is untested every single time, and the thing most
  likely to silently break (the guard itself) is the thing dev mode would
  stop running.

- **A runtime-only env var check with no compile-time gate**: read
  `APP_DEV_MODE` unconditionally in `internal/api`, no build tags at all.
  Rejected as not structurally safe enough for a mechanism that creates a
  known admin account. A real deployed binary should find it structurally
  hard to have the bypass active, not just accidentally unlikely because
  nobody happens to set the env var in production. The two-gate design
  gives this a concrete defense-in-depth property: accidentally shipping
  the bypass live requires both forgetting `-tags embedweb` (which already
  breaks the embedded frontend and serves a 501 for every page, an
  extremely visible failure, not a silent one) and separately setting
  `APP_DEV_MODE=1`. In practice these two gates are already anti-correlated:
  nobody ships `-tags embedweb` for local dev iteration, since it requires
  a fresh `npm run build` first, defeating the point of fast iteration, and
  nobody omits `-tags embedweb` for a real deployment. The failure modes
  that would have to coincide for the bypass to reach production are
  themselves mutually exclusive in normal practice.

## Consequences

- The fixed `dev`/`dev` credential pair is deliberately not randomized or
  kept secret. The build-tag gate is what makes this safe, not credential
  secrecy: once a binary is compiled without `-tags embedweb` and run with
  `APP_DEV_MODE=1`, there is no remaining protection a random password
  would add, and randomizing it would only reintroduce the friction (a
  password to look up or copy) dev mode exists to remove. This is worth
  stating explicitly so a future reader does not "fix" this by adding
  password randomization or per-run generation under the impression it is
  a gap; it is not.
- Live end-to-end verification was done against the real binary, not just
  unit tests: with `APP_DEV_MODE=1` set, the binary logs the warning and
  `dev`/`dev` returns 200 with a session cookie from the real
  `POST /api/v1/auth/login` endpoint; the same binary built the same way
  but without `APP_DEV_MODE` set returns 401 for the same `dev`/`dev`
  credentials. Separately, a real backend on `:8080` plus `npm run dev` on
  a scratch port confirmed `GET /api/v1/brand` reaches the backend through
  the new Vite proxy and returns real brand JSON. Both build variants
  (`embedweb` and not) pass `gofmt`, `go vet`, `golangci-lint`, and
  `go test -race` clean.
- This is now a second precedent in the codebase for a build-tag-gated
  file pair mirroring `embedweb`'s polarity to make a dev-only convenience
  structurally hard to ship live, alongside `web/embed_real.go`/
  `embed_stub.go` itself. Future dev-only conveniences that need the same
  structural safety property (hard to accidentally ship, not just unlikely
  to) have a working pattern to copy rather than needing to invent one.
- Because `MaybeBootstrapDevAdmin` shares `BootstrapAdmin` with the real
  env-var-driven admin bootstrap, any future change to `BootstrapAdmin`'s
  behavior (its idempotency rule, its password hashing, its account
  creation semantics) automatically applies to dev mode too, for better or
  worse; the two are not independent code paths that can silently drift
  apart, but a change intended only for the operator-facing bootstrap path
  now needs to be considered against the dev-mode path as well.
