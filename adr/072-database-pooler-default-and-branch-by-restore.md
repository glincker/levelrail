# ADR 072: Database pooler default, branch by restore and read replica scope

Status: Proposed

Date: 2026-11-02

## Context

Once an app is live, its database is usually the first thing to strain:
connection counts, a safe copy for testing, and read load. Read from
`release/v0.9.0`:

1. **Pooler.** `internal/reconcile/database/pooler.go` runs a
   pgbouncer-style container in the database's network namespace on port
   6432, `POOL_MODE=transaction`, SCRAM auth, only for Postgres
   (`PoolerActive`). It is opt-in per database through
   `POST|DELETE /api/v1/databases/{name}/pooler`. Pool size and max client
   connections come from per-database overrides, then
   `APP_PG_POOLER_DEFAULT_POOL_SIZE` and `APP_PG_POOLER_MAX_CLIENT_CONN`.
   Linked apps get a pooled `url` and a `direct_url`
   (`internal/reconcile/database/address.go`). A `pooler_saturated` alert
   kind exists (`internal/alerting/database_health.go`, default 5s wait).
2. **Restore as new.** `POST /api/v1/databases/{name}/restore-as-new`
   (`internal/api/database_clone_restore.go`) restores a given `backup_id`
   into a new database. Postgres PITR restores in place only;
   MySQL and MariaDB PITR can restore into a new database
   (`pitrRestoreMode` in `internal/api/pitr.go`, `pitr_mysql.go`).
3. **Replicas.** No managed read replica exists. Postgres already starts
   with `wal_level=replica` (`internal/reconcile/database/controller.go`),
   which a streaming replica needs.

## Decision

1. **Pooler stays off by default now.** Add runtime setting
   `databases.pooler_default_on_create` (bool, default `false`, env
   `APP_PG_POOLER_DEFAULT_ON_CREATE` seeds it, applies live). When on, a
   new Postgres database is created with the pooler enabled. It never
   changes an existing database, and a create request can still opt out.
2. **Criteria to flip the default to on**, all required, in a later
   release:
   - every Postgres template in the catalog installs and passes its health
     check through the real engine with the pooler on;
   - migrations and session features documented: linked apps use
     `direct_url` for migrations, and the pooler page lists what
     transaction mode breaks (session `SET`, server-side prepared
     statements on old drivers, advisory locks, `LISTEN/NOTIFY`, temp
     tables);
   - `pooler_saturated` measured firing on a real host, and the
     `database_connections_high` alert offers "enable pooler" as a fix;
   - one release with the setting available and no open pooler defect.
3. **Branch means restore as new.** A branch is a new database restored
   from a source, recorded with `branched_from` and `branched_at`. Phase
   one: `databases branch create <db> --name <new> [--backup <id>|latest]`
   wraps the existing restore-as-new path. Phase two: Postgres PITR into a
   new database at a target time, reusing the PITR restore code with a new
   container instead of the source. Branches get fresh credentials, never
   the source password, and are normal databases (backups, scope, delete).
   Copy on write snapshots are deferred.
4. **Read replicas: same node first, manual promote only.** A Postgres
   streaming replica runs as a child container like the pooler
   (`pg_basebackup -R`), on the primary's node, with a `ReplicaReady`
   condition, lag in status, and a `read_url` link field that falls back
   to `url` when no replica is ready. Promotion is an explicit operator
   action that stops the old primary first, promotes, then re-points
   links. No automatic failover. Cross-node replicas wait for the
   multi-node test bed (ADR 071).
5. **Thresholds from settings.** Replica lag warning
   (`databases.replica_lag_warn`, env `APP_DB_REPLICA_LAG_WARN`, default
   `30s`) and the saturation wait reuse alert rule thresholds, never
   constants.

## Alternatives rejected

- **Pooler on for every new database now.** Transaction pooling breaks
  some ORMs and session features silently; flipping before templates are
  proven turns a scaling aid into a support load.
- **Pooler on for existing databases.** Changes a working app's connection
  semantics without a deploy. Never.
- **Copy on write branches first.** Instant, but only on btrfs, ZFS or XFS
  reflink hosts. Restore works everywhere and reuses tested code. A
  filesystem probe can add the fast path later.
- **Automatic failover.** On a two-node setup split brain costs more than
  a few minutes of manual promotion, and it cannot be proven without a
  real multi-host bed.

## Security

A branch copies production data: creating one needs the same ability as
restore-as-new (`AbilityWriteSensitive`), is audited, and inherits the
source's network scope, never wider. The replica uses a dedicated
replication role, password generated and stored like owner credentials,
reachable only inside the database network. The pooler keeps SCRAM and
`server_tls_sslmode=prefer`.

## Failure modes

- Pooler down: linked apps on `url` fail while `direct_url` works; the
  existing `PoolerReady` condition reports it.
- Branch restore fails midway: the new database is left in a failed
  provision state and can be discarded; the source is never touched.
- Replica falls behind or breaks: `read_url` falls back to `url`, lag
  alert fires.
- Promote while the old primary still accepts writes: prevented by
  stopping it first; if the stop fails, promotion is refused.

## What cannot be verified locally

Pooler behavior under real client load, saturation timing, replica lag on
real disks, and anything cross-node. Docker on a laptop proves create
paths, the restore pipeline and replication setup only.

## Testing plan

Unit: create with the setting on and off; existing databases unchanged;
branch request maps to a restore-as-new job with source and time;
`read_url` resolver both ways; promote refused when the primary stop
fails. Live Docker (existing PITR gate): branch from latest backup has the
source rows; insert on primary visible on replica within the configured
lag. Templates through the real engine with the pooler on for criterion
one.

## Slices

1. Setting and create path: `internal/runtimesettings/registry_storage.go`,
   `internal/api/databases.go`.
2. Branch phase one: new `internal/api/database_branch.go`,
   `internal/apiclient/database_branch.go`, `internal/clitree/tree.go`
   (regenerate `openapi_gen.go` on conflict).
3. Postgres PITR into a new database: `internal/backup/pitr_restore.go`,
   `internal/backup/pitr_runner.go`, `internal/api/pitr.go`.
4. Replica container and condition: new
   `internal/reconcile/database/replica.go`, migration,
   `internal/api/database_replica.go`.
5. `read_url` and manual promote:
   `internal/reconcile/application/database_scope.go`,
   `internal/reconcile/database/address.go`.
   Slices 1 and 2 touch generated files and run one after the other.

## Consequences

Operators get a safe pooler switch now and a clear bar for the default,
a branch command that works on every host, and read scaling without a
failover system to trust. Branches cost minutes and bucket reads rather
than seconds, and replicas share the primary's node, so they help read
load and recovery speed, not node loss.

## Open questions

1. Should the branch create request default to `latest` backup or require
   an explicit source?
2. Should branches expire after a configurable time, and who owns the
   default?
3. Does the Postgres image's `pg_hba.conf` allow a replication role inside
   the database network as configured today, or does the reconciler need
   to write one?
4. Should MySQL and MariaDB get read replicas in the same slice set, or
   Postgres only first?
