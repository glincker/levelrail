---
title: "Upgrade compatibility"
description: "Every release must upgrade cleanly from the previous minor's latest patch, with the data it left behind. Patch releases carry no destructive migrations."
"og:url": "https://docs.levelrail.com/upgrade-compatibility"
---

Every release must upgrade cleanly from the previous minor's latest patch, with the data it left behind. Patch releases carry no destructive migrations.

## The rule

Migrations newer than the baseline in `internal/store/migrations_baseline.txt` (the highest migration shipped in the last stable release) must be additive. `go test ./internal/store` fails on:

- `DROP TABLE`, `DROP COLUMN`
- `RENAME COLUMN`, `RENAME TO` on an existing object
- `ALTER COLUMN` type changes
- `ADD COLUMN ... NOT NULL` without a `DEFAULT`

Tables created in the same migration may be dropped or renamed freely.

## Override

When a break is intended, put `-- compat-break: <reason>` on the same line as the statement. The test passes and logs every reason (`go test -v -run TestMigrationsAdditive ./internal/store`), so the owner can review them before a release.

## Upgrade matrix

Run before cutting a release (not in CI):

```
scripts/upgrade-matrix.sh [--from N] [--pre] [--binary PATH]
```

It boots each old release on a private data directory with a bootstrap admin and a few API-created rows, stops it, then boots the new binary on the same data. Per source it checks: healthy `/healthz`, `schema_migrations` max equals the newest migration, no migration or schema ERROR lines in the log, and admin login plus `GET /api/v1/apps`. Docker being unreachable is not a failure. It prints a PASS or FAIL table and exits non-zero on any FAIL.

Sources: `v0.7.0` (always), the latest 0.6.x, and the `N` latest stable releases (default 3). On Linux the release binary is downloaded and its checksum verified; on other hosts the tag is built from source, because releases ship only Linux control plane binaries. `UM_FAKE_SCHEMA=N` injects a higher schema version into each old database to confirm the matrix fails when it should.

## Bumping the baseline

When a stable release ships, set `internal/store/migrations_baseline.txt` to the highest migration number in that release (`git ls-tree --name-only <tag> internal/store/migrations/ | tail -1`).
