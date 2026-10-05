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

## Roll back

Unset `APP_AUTH_ENGINE` (or set it to `legacy`) and restart. The built-in engine never stopped being authoritative, so there is nothing to undo. The copied rows can stay; they are inert.

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
