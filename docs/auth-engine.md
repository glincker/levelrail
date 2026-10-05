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
| `APP_AUTH_ENGINE_OAUTH_PROVIDER_TTL` | `30s` | How long a resolved OAuth provider is cached. Saving provider settings drops it at once |
| `APP_AUTH_ENGINE_OAUTH_ALLOWED_HOSTS` | empty | Extra hosts allowed in an OAuth redirect URI once a dashboard URL is set |

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

- Linked OAuth identities are copied too, see [OAuth and OIDC sign-in](#oauth-and-oidc-sign-in)
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

Tokens created while shadow mode is on exist only in the built-in table, so each shows up as a `decision` mismatch until you run `levelrail auth-backfill` again.

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
- **Passkeys registered before the cutover keep working.** Their authenticators hold the built-in user handle, so Levelrail maps it to the backfilled library user and the library checks it against the credential's owner. Run `levelrail auth-backfill` first: a passkey whose user was not copied cannot sign in. No re-registration is needed.
- **Code reuse and lockout.** A TOTP code can be used once: a second use inside its 30 second window is refused (the built-in engine already did this on sign-in and disable; the library applies it to regeneration too). The library also locks a user out after repeated wrong codes (default 5 wrong codes, 15 minutes), counted per user across every device, on top of the existing per-address limit. A lockout answers 429 with `Retry-After`.
- **Sign-in with a passkey does not ask for a code**, as before.
- **A user must be copied across first.** Run `levelrail auth-backfill` after creating users, or their two-factor routes answer 409.

Roll back by unsetting `APP_AUTH_ENGINE` (or removing `mfa` from `APP_AUTH_ENGINE_AREAS`) and restarting. While the library serves MFA, enrollment and recovery codes are also written to the built-in tables, so users who enrolled or regenerated after the cutover can still sign in with an authenticator code after a rollback. Passkeys registered after the cutover are only in the library tables and must be registered again after a rollback.

## OAuth and OIDC sign-in

With `APP_AUTH_ENGINE=library` and `oauth` in `APP_AUTH_ENGINE_AREAS` (an empty list means every area), sign-in with Google, GitHub, Microsoft and your generic OIDC provider runs through the library. It needs the control plane's master key (the same one TOTP uses) and `APP_AUTH_ENGINE_BASE_URL` set to the public URL of the dashboard. Without either, OAuth quietly stays on the built-in flow.

What stays the same:

- Providers are still configured under Settings, OAuth. An edit or removal applies on the next sign-in without a restart: saving drops the cached provider immediately, and any other change is picked up within `APP_AUTH_ENGINE_OAUTH_PROVIDER_TTL`. If a provider's settings or secret cannot be read, sign-in fails closed.
- Sign-in still starts at `/api/v1/auth/oauth/{provider}/start` and ends at `/oauth/complete`. Failures still land on `/login?oauth_error=<code>` with the same codes.
- The browser binding cookie keeps its rules: HttpOnly, SameSite Lax, ten minutes, Secure on HTTPS, cleared on use. PKCE (S256) and the state check are unchanged.
- A provider's allowed email domain still gates new accounts only. With it empty, any user the provider verifies may sign up with the `read` ability. Existing linked identities always sign in.
- Linking a provider to an account from your profile still uses the built-in flow (same callback URL). Identities linked this way are copied into the library as well.

### Callback URLs stay the same

The library is told to use the built-in redirect URI, `https://<host>/api/v1/auth/oauth/{provider}/callback`, built from the request host exactly as before. No provider needs a new redirect URI, and a rollback needs none either. The link flow keeps using the same callback: Levelrail tells the two apart by the state value.

Once a dashboard URL is set (Settings, Ingress), redirect URIs are limited to that host and the base URL host. Add other hostnames you sign in from with `APP_AUTH_ENGINE_OAUTH_ALLOWED_HOSTS` (comma separated, `host` or `host:port`). With no dashboard URL, any request host is accepted, as in the built-in flow.

### Behavior changes

- An existing account is now linked when the provider reports the email as verified. The built-in flow refused every sign-in whose email matched an existing account. An unverified email (Microsoft never reports one) is still refused.
- A generic OIDC issuer must use `https`.
- Microsoft accounts keep the Graph object id they were stored under, so existing links keep working.

### Existing identities

`levelrail auth-backfill` copies each linked identity (provider and provider user id) to the same user, so people keep signing in with their provider after the cutover. The built-in flow never stored provider tokens, so nothing else exists to copy. Run it before enabling the area. A user created by a first OAuth sign-in through the library also gets a built-in identity, so a rollback keeps them signed in.

### Roll back

Remove `oauth` from `APP_AUTH_ENGINE_AREAS` (or unset `APP_AUTH_ENGINE`) and restart. Sign-in returns to the built-in flow, which uses the same callback URLs.

## Rehearsing the migration

Rehearse the backfill on realistic data before you turn the library engine on. The rehearsal builds a synthetic legacy database (users with bcrypt hashes, mixed API tokens, passkeys, two-factor enrolments), runs the real `auth-backfill` command against it, and then checks that people and tokens still get in.

### Run it locally

```
scripts/auth-rehearsal/run.sh
```

This needs Go and nothing else. It seeds 2000 users, 5000 API tokens, 300 passkeys and 400 two-factor users into a temporary directory, then runs these checks in order:

1. The backfill refuses duplicate emails that differ only by case, writing nothing. The rehearsal then renames them and continues.
2. `--dry-run` prints counts that match the seed and writes nothing.
3. A failure injected midway (a bad token, then a bad passkey) rolls back to zero library rows and leaves the legacy tables untouched.
4. The real run commits in one transaction (a concurrent reader only ever sees 0 or all users), prints counts that match the seed, and changes no legacy table.
5. A second run copies nothing and changes no library table.
6. Parity: a sample of users sign in through the library (bcrypt accepted, hash upgraded to Argon2id on the first sign-in and used as is on the second), real TOTP codes verify, passkeys read back with the same credential id and public key, live tokens authenticate with the right owner and abilities, and expired, revoked and orphaned-owner tokens are rejected.
7. Rollback drill: with the engine set back to `legacy` (and with single areas switched off through `APP_AUTH_ENGINE_AREAS`), legacy sign-in and token auth work on the same database.

Settings, all optional: `APP_REHEARSAL_SCALE` (default 10, the multiple used for the extra timing run, 0 skips it), `APP_REHEARSAL_USERS`, `_TOKENS`, `_PASSKEYS`, `_TOTP_USERS`, `_CASE_DUPES`, `_OAUTH`, `_SEED`, `_SAMPLE_USERS`, `_SAMPLE_TOKENS`, and `APP_REHEARSAL_DIR` to keep the databases. The same seed always builds the same data. To build a database without running the checks: `go run ./scripts/auth-rehearsal/seed -dir /tmp/rehearsal-data`.

### Run it against a copy of production data

Never point the rehearsal, or any dry run you have not read, at the live database. Take a consistent copy first:

```
sqlite3 /var/lib/levelrail-data/levelrail.db ".backup '/srv/rehearsal/levelrail.db'"
cp /var/lib/levelrail-data/master.key /srv/rehearsal/master.key
```

Then run the real command against the copy by pointing the data directory at it:

```
APP_DATA_DIR=/srv/rehearsal levelrail auth-backfill --dry-run
APP_DATA_DIR=/srv/rehearsal levelrail auth-backfill
APP_DATA_DIR=/srv/rehearsal levelrail auth-backfill
```

The first run shows exact counts and what would be refused. The second applies it to the copy. The third must report zero copied. If you keep the master key out of the data directory, set `APP_MASTER_KEY` instead of copying the file. Delete the copy afterwards, it holds your password hashes.

### What to look at

- Counts: users, password hashes, tokens, passkeys and TOTP secrets should equal what you expect from your own instance. Tokens skipped for having no owner are system tokens and tokens whose owner was deleted.
- Peak memory and wall time, printed as `TIMING`. The backfill loads rows into memory, so expect both to grow with the row count; compare the default and scaled figures.
- A refusal about colliding emails: rename the later account of each pair before the real run.
- The recovery code note: every two-factor user must regenerate recovery codes after cutover.

### Go or no-go checklist

- The dry run on a copy of production prints counts you can explain.
- The real run on the copy succeeds and a second run copies nothing.
- No refusal remains (duplicate emails resolved, no unknown abilities).
- Sign-in works through the library for an admin, an ordinary user, a two-factor user, and a user with a passkey registered before the cutover on the copy.
- A live API token and a revoked one behave correctly through the library on the copy.
- You have a recent backup of the real data directory and you know the rollback: unset `APP_AUTH_ENGINE`, or narrow `APP_AUTH_ENGINE_AREAS`, and restart.
- You have told two-factor users they must regenerate recovery codes.
