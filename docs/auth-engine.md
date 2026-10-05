---
description: Opt in to the library auth engine, copy existing accounts and tokens across, and roll back by unsetting one variable.
---

# Library auth engine (preview)

The control plane can run a second, library-backed auth engine beside the built-in one. This is a preview: it is off by default, and turning it on changes nothing about how you sign in today. It mounts a separate set of routes so you can check that your existing accounts and API tokens work through it before any cutover.

## Enable it

Set one environment variable on the control plane and restart:

```
APP_AUTH_ENGINE=library
```

The library routes appear under `/api/v1/auth-lib`. Set `APP_AUTH_ENGINE_PATH_PREFIX` to use a different prefix. The current sign-in, tokens and sessions keep working on their existing routes.

With the variable unset (or `legacy`), nothing is mounted and the new tables are never read or written. The tables are created by an upgrade migration either way, and stay empty.

| Variable | Default | Purpose |
| --- | --- | --- |
| `APP_AUTH_ENGINE` | `legacy` | `library` mounts the library routes |
| `APP_AUTH_ENGINE_PATH_PREFIX` | `/api/v1/auth-lib` | Route prefix |
| `APP_AUTH_ENGINE_BASE_URL` | derived from the listen address | Public URL used in generated links |
| `APP_AUTH_ENGINE_PASSWORD_MIN_LENGTH` | `8` | Minimum password length |
| `APP_AUTH_ENGINE_DEVICE_POLL_INTERVAL` | `5s` | Device login poll interval |
| `APP_AUTH_ENGINE_RATE_LIMIT_PER_IP` | library default | Per-IP request budget |

## Copy your accounts across

Run the backfill on the control plane host. Start with a dry run, which does the same work and then rolls it back, so the counts are exact:

```
levelrail auth-backfill --dry-run
levelrail auth-backfill
```

It copies, in one transaction:

- users, with a mapping from your existing ids to the library's ids
- password hashes, verbatim. Hashes are re-encrypted to the library's format the first time each user signs in through the library
- API tokens with an owner. Existing tokens keep working as they are (no prefix needed) and keep their abilities
- passkeys
- TOTP secrets, re-encrypted with a key held in the control plane's secret store

Running it again is safe: rows already copied are skipped. Output is counts only, never secrets.

Things to know:

- Recovery codes are not converted. Users with two-factor enabled must regenerate them.
- API tokens with no owner (created by the system itself) are not copied.
- Emails that differ only by letter case count as duplicates in the library. The backfill stops and tells you how many collide so you can rename them first.
- A token's effective abilities are limited to what its owner currently holds, so a token can never exceed its owner.

## Sessions and login

With `APP_AUTH_ENGINE=library` and `sessions` in `APP_AUTH_ENGINE_AREAS` (or the list left empty), password login, sessions, first-run registration, session links, password reset and stream re-authorization are served by the library. The dashboard and CLI see the same routes, request bodies, status codes and session cookie as before, so nothing changes for users or scripts.

What moves:

- Sign-in, sign-out and "current session" read and write sessions in the database instead of server memory.
- Login throttling and per-account lockout come from the library. Lockouts are stored in the database, so they survive a restart.
- First-run registration still needs the setup token. The library closes sign-up after the first account.
- Session links and password reset tokens are created and checked by the library. A session link is re-checked against its user on every use.
- Live streams (logs, deploy progress, terminal) close within `APP_AUTH_ENGINE_STREAM_WATCH_INTERVAL` of the session ending, on top of the existing periodic check.
- Sign-in, failed sign-in, password change and sign-out events appear in the audit log with the same paths they had before.

Your users, roles, invites and abilities stay in Levelrail. A user created, deleted or given a new password in Levelrail is mirrored into the library at once. Deleting a user ends their sessions immediately.

**One-time sign-out.** Built-in sessions only lived in memory, so switching the mode signs everyone out once. Everyone signs in again with the same password.

Settings (all optional, defaults match the built-in behavior):

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_SESSION_TTL` | `24h` | Absolute session lifetime |
| `APP_AUTH_ENGINE_SESSION_IDLE_TIMEOUT` | `0` (off) | End a session unused for this long |
| `APP_AUTH_ENGINE_SESSION_LINK_TTL` | `2m` | How long a session link can be exchanged |
| `APP_AUTH_ENGINE_LOGIN_GRACE_FAILURES` | `3` | Free failed logins per IP and username before backoff |
| `APP_AUTH_ENGINE_LOGIN_BASE_DELAY` | `1s` | First backoff delay, doubled per further failure |
| `APP_AUTH_ENGINE_LOGIN_MAX_DELAY` | `15m` | Backoff cap |
| `APP_AUTH_ENGINE_LOGIN_RESET_AFTER` | `15m` | Forget failures after this idle time |
| `APP_AUTH_ENGINE_LOGIN_USER_MAX_FAILURES` | `10` | Consecutive failures on one account, from any IP, that lock it |
| `APP_AUTH_ENGINE_LOGIN_USER_LOCKOUT` | `15m` | How long that lock lasts |
| `APP_AUTH_ENGINE_RATE_LIMIT_PER_IP` | `60` | Credential requests per minute per IP |
| `APP_AUTH_ENGINE_RATE_LIMIT_PER_EMAIL` | `60` | Credential requests per minute per username |
| `APP_AUTH_ENGINE_STREAM_WATCH_INTERVAL` | `5s` | How often a live stream re-checks its session |

Account recovery: `recover-admin` sets the password through the library, which also ends that account's sessions and clears its lockout and login backoff. No restart is needed.

Things to know:

- Run `auth-backfill` before switching, and avoid changing passwords between the backfill and the switch. A stale copy is refreshed at startup when it still holds the old hash.
- Sign-in is case-insensitive on the username in library mode.
- A session link minted from an API token (rather than a user session) still uses the built-in pinned session, because the library ties links to users. User-minted links use the library.
- A first-run username that is not an email address (for example `admin`) is still accepted: the account is created by Levelrail and mirrored, still behind the setup token.
- Multi-factor sign-in and passkeys are separate areas. While `mfa` is not served by the library, a user with two-factor enabled still gets the usual second step.

Roll back by removing `sessions` from `APP_AUTH_ENGINE_AREAS`, or by unsetting `APP_AUTH_ENGINE`, and restarting. Built-in sessions start empty, so everyone signs in once more. Passwords keep working because Levelrail's own copy is kept up to date.

## Roll back

Unset `APP_AUTH_ENGINE` (or set it to `legacy`) and restart. The built-in engine never stopped being authoritative, so there is nothing to undo. The copied rows can stay; they are inert.

## Shadow mode and token cutover

`APP_AUTH_ENGINE` takes three values: `legacy` (default), `shadow` and `library`.

### Shadow mode

```
APP_AUTH_ENGINE=shadow
```

The built-in engine serves every request exactly as before. For each request that presents a bearer API token, the library also authenticates the same token in the background and records whether it agrees on three things: accept or reject, the owning user, and the effective abilities. Nothing is mounted, nothing is slowed (comparisons run on a small bounded queue and are dropped, with a counter, when it is full), and no secret is ever logged or stored in the results.

Check it:

```
levelrail-cli auth-engine status
```

The same data is on Settings > Security (root only) and at `GET /api/v1/auth-engine/status`: mode, library version, counts (compared, matched, mismatched, dropped, skipped, errors) and the most recent mismatches (token id, owner id, kind). Tokens created by the system itself have no owner, are never copied across, and are counted as skipped. Run the backfill first, or every owned token shows up as a `decision` mismatch.

| Variable | Default | Purpose |
| --- | --- | --- |
| `APP_AUTH_ENGINE_SHADOW_QUEUE` | `256` | Pending comparisons before new ones are dropped |
| `APP_AUTH_ENGINE_SHADOW_WORKERS` | `2` | Comparison goroutines |
| `APP_AUTH_ENGINE_SHADOW_MISMATCH_LOG` | `50` | Mismatches kept for the status view |

A library token's abilities are limited to what its owner holds right now, so a mismatch of kind `abilities` usually means a user lost an ability after minting the token. The built-in engine does not clamp.

### Cut tokens and device login over

```
APP_AUTH_ENGINE=library
APP_AUTH_ENGINE_AREAS=tokens,device
```

Bearer authentication, the token routes (`/api/v1/auth/tokens`) and the device login routes (`/api/v1/auth/device/*`) then run on the library. Request and response shapes and status codes are unchanged. Sign-in, sessions and everything else stay on the built-in engine.

- Copied tokens keep working with no prefix. New tokens carry the library prefix. Tokens that never expire still never expire.
- Every token created in this mode is also written to the built-in table, so rolling back loses nothing.
- A token with an owner that the backfill has not copied is refused (a warning names its id): run `levelrail auth-backfill` first. System tokens with no owner keep working.
- Device login keeps `APP_DEVICE_TOKEN_TTL_DAYS`, the one-time redeem, and the cap that stops a root approver minting a root token unless `APP_DEVICE_TOKEN_ALLOW_ROOT=true`. Differences: the token is named `device: <client>` instead of `cli login: <client>`, and its abilities are fixed when the request is approved rather than when it is collected.

### Roll back

Set `APP_AUTH_ENGINE` to `legacy` (or `shadow`) and restart. Tokens revoked while on the library are revoked in the built-in table too. No data is lost.

## MFA (TOTP and passkeys)

With `APP_AUTH_ENGINE=library` and `mfa` in `APP_AUTH_ENGINE_AREAS` (or the list left empty, which means every area), two-factor codes and passkeys are served by the library on the same routes, with the same request and response shapes, so the dashboard and CLI need no changes. With either setting off, nothing below applies and the built-in handlers run exactly as before.

What moves:

- TOTP setup, confirm, sign-in verification, disable, status and recovery codes
- Passkey registration, sign-in, list and delete, plus a new rename (`PATCH /api/v1/auth/passkeys/{id}` with `{"label": "..."}`)
- Your existing authenticator entries keep working: the backfill copies each TOTP secret re-encrypted, and the algorithm is unchanged

| Variable | Default | Purpose |
| --- | --- | --- |
| `APP_AUTH_ENGINE_WEBAUTHN_RP_ID` | host of the dashboard URL | Passkey relying party ID |
| `APP_AUTH_ENGINE_WEBAUTHN_ORIGINS` | the dashboard URL origin | Extra allowed origins, comma separated |
| `APP_AUTH_ENGINE_WEBAUTHN_REQUIRE_UV` | `false` | Demand PIN or biometric on every passkey use |
| `APP_AUTH_ENGINE_WEBAUTHN_CLONE_WARNING` | `reject` | `reject` refuses a sign-in whose counter went backwards, `flag` allows it and records an audit event |
| `APP_AUTH_ENGINE_MFA_MAX_FAILURES` | `5` | Wrong codes per user before a lockout |
| `APP_AUTH_ENGINE_MFA_LOCKOUT` | `15m` | How long that lockout lasts |

Things to know:

- **Recovery codes need regenerating.** Legacy codes cannot be converted, so after the cutover every user with two-factor on has none. The first time they open the dashboard a notice asks them to generate a new set (Settings, Security). Their authenticator app keeps working in the meantime. The 2FA status response carries `recovery_codes_need_regeneration` while this applies.
- **Passkey relying party comes from configuration, not the request.** The library fixes the relying party ID at startup. It is derived from the dashboard URL (Settings, Ingress) when the control plane starts, so changing that URL needs a restart. An IP address cannot be a relying party ID: set the dashboard URL to a domain, or set `APP_AUTH_ENGINE_WEBAUTHN_RP_ID`. Without one, passkey routes answer 501.
- **Passkeys registered before the cutover can be listed, renamed and deleted but cannot sign in.** The library identifies the account from a user handle stored on the authenticator, and passkeys made by the built-in engine store a different one. Register the passkey again from Settings, Security.
- **Code reuse and lockout.** A TOTP code can be used once: a second use inside its 30 second window is refused (the built-in engine already did this on sign-in and disable; the library applies it to regeneration too). The library also locks a user out after repeated wrong codes (default 5 wrong codes, 15 minutes), counted per user across every device, on top of the existing per-address limit. A lockout answers 429 with `Retry-After`.
- **Sign-in with a passkey does not ask for a code**, as before.
- **A user must be copied across first.** Run `levelrail auth-backfill` after creating users, or their two-factor routes answer 409.

Roll back by unsetting `APP_AUTH_ENGINE` (or removing `mfa` from `APP_AUTH_ENGINE_AREAS`) and restarting. While the library serves MFA, enrollment and recovery codes are also written to the built-in tables, so users who enrolled or regenerated after the cutover can still sign in with an authenticator code after a rollback. Passkeys registered after the cutover are only in the library tables and must be registered again after a rollback.
