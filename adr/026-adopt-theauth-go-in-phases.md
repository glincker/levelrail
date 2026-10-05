# ADR 026: Adopt theauth-go in phases, superseding ADR 010

Status: Accepted

Date: 2026-10-05

## Context

ADR 010 declined theauth-go for three concrete reasons: no SQLite adapter, a
monolithic `Storage` interface, and a chi-only `Mount`. All three are now
resolved in the library (`storage/sqlite`, capability-split `CoreStorage`,
`Handler() http.Handler` with a configurable `PathPrefix`). The remaining
cost is data and contract migration, not library plumbing: user ids differ
(ULID vs `user_<hex>`), API tokens are unprefixed, timestamps and hash
encodings differ.

## Decision

Adopt the library in phases, each shippable on its own.

1. Slice 1 (this ADR's first PR): `internal/authengine` behind
   `APP_AUTH_ENGINE` (`legacy` default, `library`). Library tables arrive in
   migration 0372 and stay empty while the flag is off. When on, the library
   handler mounts on a separate prefix and the legacy routes are untouched.
   `auth-backfill` copies users, bcrypt hashes, API tokens, passkeys and TOTP
   secrets with an id map. Parity tests prove legacy credentials work through
   the library.
2. Later slices: shadow comparison, token and device flow cutover, session
   login, TOTP, passkeys, OAuth, then deletion of the hand-rolled code.

## Rejected alternatives

- Keep the hand-rolled auth. Cheapest now, but it keeps carrying S1 to S10
  style gaps the library already closes (PKCE state hardening, persistent
  login throttle, clone detection) and every feature is built twice.
- Big-bang cutover. 60 to 80 files and several thousand lines in one change,
  with no way to compare behavior on real data first. Phasing with a flag and
  a rehearsable backfill keeps rollback to "unset the flag".

## Consequences

- Recovery codes are not convertible (different hash layout); users with
  TOTP must regenerate them at cutover.
- Backfill writes the library tables in one transaction over the single
  SQLite connection, so it does not use the library's per-call storage API.
  The parity tests guard that coupling.
- Both engines coexist until the final slice, so the binary carries the
  library's dependencies (SAML, XML) even with the flag off.
