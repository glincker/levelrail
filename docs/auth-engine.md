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

## Roll back

Unset `APP_AUTH_ENGINE` (or set it to `legacy`) and restart. The built-in engine never stopped being authoritative, so there is nothing to undo. The copied rows can stay; they are inert.

## OAuth and OIDC sign-in

With `APP_AUTH_ENGINE=library` and `oauth` in `APP_AUTH_ENGINE_AREAS` (an empty list means every area), sign-in with Google, GitHub, Microsoft and your generic OIDC provider runs through the library. It needs the control plane's master key (the same one TOTP uses) and `APP_AUTH_ENGINE_BASE_URL` set to the public URL of the dashboard. Without either, OAuth quietly stays on the built-in flow.

What stays the same:

- Providers are still configured under Settings, OAuth. An edit or removal applies on the next sign-in without a restart: saving drops the cached provider immediately, and any other change is picked up within `APP_AUTH_ENGINE_OAUTH_PROVIDER_TTL`. If a provider's settings or secret cannot be read, sign-in fails closed.
- Sign-in still starts at `/api/v1/auth/oauth/{provider}/start` and ends at `/oauth/complete`. Failures still land on `/login?oauth_error=<code>` with the same codes.
- The browser binding cookie keeps its rules: HttpOnly, SameSite Lax, ten minutes, Secure on HTTPS, cleared on use. PKCE (S256) and the state check are unchanged.
- A provider's allowed email domain still gates new accounts only. With it empty, any user the provider verifies may sign up with the `read` ability. Existing linked identities always sign in.
- Linking a provider to an account from your profile still uses the built-in flow and its callback URL. Identities linked this way are copied into the library as well.

### Callback URLs change

The library builds the redirect URI as `{APP_AUTH_ENGINE_BASE_URL}{APP_AUTH_ENGINE_PATH_PREFIX}/providers/{provider}/callback`. The built-in flow uses `https://<host>/api/v1/auth/oauth/{provider}/callback`. The library cannot produce the old path, so each provider needs one more redirect URI before you enable the area:

1. At each identity provider, add `https://<your-host>/api/v1/auth-lib/providers/<provider>/callback` (use your own prefix if you changed it).
2. Keep the old `/api/v1/auth/oauth/<provider>/callback` entry. The link flow and a rollback still use it.
3. Enable the area and restart.

### Behavior changes

- An existing account is now linked when the provider reports the email as verified. The built-in flow refused every sign-in whose email matched an existing account. An unverified email (Microsoft never reports one) is still refused.
- A generic OIDC issuer must use `https`.
- Microsoft accounts keep the Graph object id they were stored under, so existing links keep working.

### Existing identities

`levelrail auth-backfill` copies each linked identity (provider and provider user id) to the same user, so people keep signing in with their provider after the cutover. The built-in flow never stored provider tokens, so nothing else exists to copy. Run it before enabling the area. A user created by a first OAuth sign-in through the library also gets a built-in identity, so a rollback keeps them signed in.

### Roll back

Remove `oauth` from `APP_AUTH_ENGINE_AREAS` (or unset `APP_AUTH_ENGINE`) and restart. Sign-in returns to the built-in flow and its callback URLs, which you kept registered.
