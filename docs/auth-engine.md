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
