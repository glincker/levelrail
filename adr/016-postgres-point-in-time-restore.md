# ADR 016: Point-in-time restore for Postgres

Status: Accepted

Date: 2026-09-22

## Context

Restore was entirely snapshot-based: `internal/api/restore.go` and
`database_clone_restore.go` restore from a specific `backup_history` row
(a `pg_dump`/`mysqldump` logical dump), recoverable only to the exact
moment that dump was taken. Postgres and MySQL both natively support
continuous write-ahead log (WAL/binlog) archiving, which makes restore
to an arbitrary timestamp possible, not just to whenever a backup
happened to run.

True PITR needs a base backup (a full copy of the data directory at a
known point) plus continuously archived WAL to replay forward from it.
Neither existed. Scoped to Postgres only for this pass: MySQL binlog
PITR is a real follow-up, not implemented here.

## Decision

**Physical base backup via `pg_basebackup`, not the low-level
`pg_backup_start`/`pg_backup_stop` API.** `pg_basebackup --format=tar`
embeds `backup_label` (and `tablespace_map`) inside its own tar output,
so a restore needs nothing captured or shipped alongside it. The
lower-level API would require manually assembling that tar and
separately capturing `backup_label`'s content, more surface for a
mistake in the single most correctness-sensitive path this feature has.

**WAL archiving is opt-in per database** (`DesiredDatabase.PITREnabled`),
never retroactive. Enabling it flips the Postgres container's command to
add `wal_level=replica`, `archive_mode=on`, and `archive_command`
copying into a dedicated `wal-archive` Docker volume
(`internal/reconcile/database/controller.go`). Only WAL written after
enabling is ever recoverable by timestamp.

**A restore replaces the container's own data volume, not a fresh
one.** Unlike a logical restore (dump piped into `psql` against a
running container), Postgres cannot have its data directory replaced
while its own server process is running against it. The container must
be stopped, its data volume wiped and repopulated from the base backup,
recovery configured, then started again.

**`DesiredDatabase.Suspended` is the race guard for that window**, not a
new mechanism. While `Suspended` is true, `Controller.Reconcile`'s
existing branch tears the container down and takes no other action on
every pass, so nothing can race a `PITRRunner` mid-wipe. Clearing it
again lets the reconciler recreate the container through its own
existing desired-state-driven spec (image, env, PITR command flags,
volume mounts) rather than `internal/backup` hand-rolling a second copy
of that spec that could drift from it.

**A restore's "did it actually work" signal is `pg_is_in_recovery()`
returning false, polled directly, not the reconciler's Ready
condition.** Ready only reflects "the container process is running,"
which stays true even if Postgres is stuck retrying `restore_command`
for a WAL segment that will never arrive (a target timestamp beyond
what's actually archived does not fail, it hangs). `PITRRunner` polls
Postgres directly and times out as a failure rather than ever reporting
a restore succeeded because the container merely stayed up.

**`pitr_restore_history` is a separate table from `restore_history`,**
not new columns on it. `restore_history.backup_history_id` is `NOT NULL`
with a foreign key to `backup_history(id)`; a PITR restore has no single
`backup_history` row to point at (a `base_backup_history` row plus an
arbitrary timestamp instead). Rebuilding that already-shipped table to
relax the constraint (SQLite has no `ALTER COLUMN`) carries real risk
for no benefit over an additive second table with its own FK to
`base_backup_history`.

**Base backups are manual-trigger only in this pass**, not scheduled.
Automatic periodic base backups are a real follow-up; documented as a
known gap (`docs/managing-databases.md`), not silently missing.

## Consequences

- Restoring to any point requires PITR to have been enabled before that
  point and a succeeded base backup at or before it.
- The window a restore can target (`GET .../pitr`) is computed live,
  forcing a WAL segment switch on the real container so the reported
  upper bound is provably archived, not a stale cached value.
- Verified end to end against a real Postgres container, real WAL
  archiving, a real base backup, and a real S3-compatible bucket: data
  written before a chosen timestamp survives a restore to that
  timestamp, data written after it does not
  (`internal/backup/pitr_live_test.go`,
  `test/e2e/pitr_test.go`).

## Rejected alternatives

- **Manual `pg_backup_start`/`pg_backup_stop` base backups**: more code,
  more ways to get the label/tablespace-map handling wrong, no benefit
  over `pg_basebackup` doing the identical thing correctly already.
- **Rebuilding `restore_history` to support a nullable
  `backup_history_id`**: real schema-migration risk on an already-shipped
  table for a need a second, additive table meets with none.
- **Trusting the reconciler's Ready condition as restore success**: it
  cannot distinguish "container running, Postgres promoted" from
  "container running, Postgres stuck replaying WAL forever."
