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
- Sign-in works through the library for an admin, an ordinary user, and a two-factor user on the copy.
- A live API token and a revoked one behave correctly through the library on the copy.
- You have a recent backup of the real data directory and you know the rollback: unset `APP_AUTH_ENGINE`, or narrow `APP_AUTH_ENGINE_AREAS`, and restart.
- You have told two-factor users they must regenerate recovery codes.
