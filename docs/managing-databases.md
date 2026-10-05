---
description: Create, manage, back up, and restore managed databases across multiple engines (Postgres, Redis, MySQL, MongoDB, and more).
---

# Managing databases

Create a Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB, Dragonfly, or ClickHouse database with one API call or `levelrail-cli databases create`, and Levelrail runs it, generates its credentials, and backs it up on a schedule, the same way it manages an app, minus the build step and domain routing a database doesn't need.

::: details For contributors: where this lives in the source
- `internal/api/databases.go` - API handlers
- `internal/reconcile/database` - the database controller
- `internal/backup` - backup, restore, and verification
- `internal/store` - the `database_engines.yaml` registry and the desired-state schema
:::

## Why a database is not "just an app"

An app in this platform is a container plus a build plus routing plus
scaling. A database needs almost none of that: it needs a data volume that
survives redeploys, a generated credential nobody types in by hand, and
(for two engines) TLS on by default. Modeling it as its own resource
instead of an app with a special flag keeps the app controller from
growing engine-specific branches (how does a rolling deploy even apply to
a stateful Postgres container?) and keeps the database controller from
carrying build/routing logic it will never use.

Engine support is a registry, not a hardcoded switch in the API layer.
`internal/store/database_engines.yaml` lists the eight supported engines
(id, display label, default version); `internal/api/database_engines.go`
serves it at `GET /api/v1/database-engines` so the dashboard's creation
wizard and the CLI's `--interactive` flow read the live list instead of
duplicating it as TypeScript or Go literals. Adding a ninth engine to the
registry without a matching case in
`internal/reconcile/database/controller.go`'s per-engine switch would
advertise something this control plane can't actually run, so
`database_engines_test.go`'s `TestSupportedEngines_MatchReconcilerCases`
cross-checks the two never drift apart.

The eight engines today: `postgres`, `redis`, `mysql`, `mongodb`,
`mariadb`, `keydb`, `dragonfly`, `clickhouse`.

## How it actually works

**Creating a database**

`POST /api/v1/databases` writes the desired state (name, engine, version) and returns immediately. The reconciler actually starts the container on its next pass. If credentials are needed but don't exist yet, the reconciler refuses to start and reports this as a real condition.

Check status with `GET /api/v1/databases/{name}/status` or `databases get` to see if the database is actually running.

Every database you create shows up alongside your apps on its own list page, with engine, version, and status at a glance:

![Levelrail databases list showing engine, version, and running status](assets/screenshots/databases-list.png)

**Placement**

Omit `node_id` at creation to let simple spread scheduling pick a node. Pass `node_id` explicitly to override it. To move an already-created database to a different node, call `PUT /api/v1/databases/{name}/node` (gated at the root ability tier, since node placement is fleet-level infrastructure).

**Stop, start, and delete**

- **Stop and start**: Flip a `suspended` flag without touching desired state or the data volume. The reconciler removes or recreates the container on its next pass against the exact same volume.
- **Delete**: `DELETE /api/v1/databases/{name}` stops and removes the container and the desired-state row. The data volume is kept, so a mistaken delete never destroys data (see [Stop, start, and delete](#stop-start-and-delete-dashboard-and-cli)). It answers `409` while an app still connects to the database, unless you pass `force=true`.

## TLS: on by default, for two engines, with no toggle

Postgres and Redis get TLS enabled automatically at creation time with a self-signed certificate generated once and persisted through the secrets mechanism. There is no operator toggle: TLS shows as on the moment a certificate exists for the database, and off otherwise, it's not a setting you flip.

**Why only Postgres and Redis**

Both engines expose an "encrypt without verifying" mode in their standard connection URI:

- **Postgres**: `sslmode=require`
- **Redis**: `rediss://`

Mainstream client libraries honor these with zero app-side code changes.

MySQL/MariaDB lack a driver-agnostic URI knob for this. MongoDB's equivalent exists but isn't wired up yet. KeyDB accepts Redis's TLS flags unchanged (checked against `eqalpha/keydb`) but this is not wired up yet. Dragonfly refuses to start with TLS and no authentication (`TLS configured but no authentication method is used`), and this platform configures no database passwords for Redis-family engines, so it cannot be enabled without a larger change. ClickHouse is not wired.

The certificate is self-signed and never distributed to a party that verifies its issuer. It is valid for ten years. Rotation is a deliberate future operator action, not forced by expiry.

The certificate is ECDSA P-256, not ed25519: libpq's SCRAM channel binding fails on an ed25519 certificate (`could not find digest for NID UNDEF`), which broke `psql`, psycopg2 and every other libpq client using the default `sslmode=require` URL. Databases created by an earlier release are given a new certificate and recreated once, automatically, the next time the reconciler sees them.

**In apps**

When an app attaches to a TLS-enabled database, it automatically gets the TLS-flavored connection string: `?sslmode=require` appended for Postgres, or switched to `rediss://` on the TLS-only port for Redis. Nothing in `app.yaml` opts into this; it reflects the database's state.

A database's own Overview tab shows its connection details, resource usage, and the public access and backup cards covered below:

![Levelrail database overview page with connection info and resource usage](assets/screenshots/database-overview.png)

## Resource limits

Set memory, CPU, swap, and CPU pin (`cpuset`) using `PUT /api/v1/databases/{name}/resources`. Set to `null` or omit to clear limits.

When the database has a running container, new limits apply live. If the container does not exist yet or the node is unreachable, limits apply on the next restart. The response's `resources_applied_live` field tells you which happened.

**Resource recommendations**

Call `GET /api/v1/databases/{name}/resource-recommendation` to get memory/CPU suggestions based on the database's historical usage (same feature as apps).

**Dashboard**

Use the **Resources** tab (`web/src/routes/databases/$name/resources.tsx`), which includes a recommendation card above the limits editor.

## Public access: exposing a database port directly

By default, managed databases are reachable only from the platform's Docker network. Use `PUT /api/v1/databases/{name}/public-access` to bind to a host port so you can access it with pgAdmin, TablePlus, RedisInsight, or other tools from your machine.

**Port selection**

- Leave `port` at `0` or omit it to auto-assign the next free port.
- Request a specific port in the `1024-65535` range.
- Reserved ports (`80`, `443`, `8080`, `9443`) are rejected outright.
- Changing the port while already public is a state change. The dashboard port field is only editable before you enable access.

**Bind address**

Use `bind_address` to choose which network interface the port binds to:

- `private`: Loopback only (default when omitted)
- `public`: Every interface (explicit opt-in)
- A literal IP address

See [app-spec-reference.md's Bind addresses and exposure](app-spec-reference.md#bind-addresses-and-exposure) for the full table of rules (`internal/bindaddr.Resolve`).

A database already publicly accessible before this field existed keeps that exposure (backfilled to `public`). Re-enabling access later without an explicit `bind_address` picks up the new `private` default.

**Clearing public access**

`DELETE /api/v1/databases/{name}/public-access` reverts to internal-only. The reconciler replaces the running container without the host port binding on its next pass.

**Passwordless engines warning**

Redis, KeyDB, and Dragonfly run passwordless by default. Publishing their port means unauthenticated read/write access to the whole dataset. This is a materially different risk than Postgres/MySQL's generated passwords.

The dashboard gates the toggle behind an explicit "I understand this database has no password" checkbox for these engine families. Postgres/MySQL/MongoDB/MariaDB/ClickHouse get a plain, non-blocking warning instead.

**Dashboard and CLI**

- Dashboard: **Public access** card on the database's Overview tab (`web/src/components/DatabasePublicAccessCard.tsx`)
- CLI: `levelrail-cli databases public-access set <name> [--port N] [--bind-address ADDR]` and `levelrail-cli databases public-access clear <name>` (`cmd/levelrail-cli/databases_public_access.go`)
- Also reachable through `databases create --interactive`'s wizard (called as a create-time follow-up, always at the default bind address)

## Attaching a database to an app

Attach an already-created database to an app using `PUT /api/v1/apps/{name}/database`. The reconciler injects the resolved value as an env var the next time the app's container is created.

You don't need to deploy from an `app.yaml` with a `{ from: "<database>.<field>" }` env var to do this.

This flow attaches at most one database per app. If an app needs more than one, or you want to see cross-node reachability before connecting, use [Connecting apps to databases](connecting-apps-to-databases.md) instead, a newer, separate mechanism that allows arbitrarily many connections per app.

**Fields**

- `database_name`: Required
- `env_var`: Defaults to `DATABASE_URL`
- `field`: Defaults to `url` (full connection string)

The common case needs only one call. Other resolvable fields are `host`, `port`, `username`, `password`, and `database`.

All fields are validated against `database.SupportsField`'s engine-aware rules before saving. Redis, KeyDB, and Dragonfly don't support `username` or `database` fields (they don't model these concepts).

**Detaching**

`DELETE /api/v1/apps/{name}/database` stops injecting the env var into newly created containers. It does not retroactively touch containers already running.

**Dashboard**

The **Database** card on an app's Overview page (`web/src/components/DatabaseAttachmentCard.tsx`) includes an "Env var / field options" disclosure for anything past the URL default. This lives on the app's page, not the database's, since the attachment is the app's env var source.

## Stop, start, and delete: dashboard and CLI

::: code-group
```bash [CLI]
levelrail-cli databases stop <name>
levelrail-cli databases start <name>
levelrail-cli databases delete <name> [--force]
```
```bash [API]
POST /api/v1/databases/{name}/stop
POST /api/v1/databases/{name}/start
DELETE /api/v1/databases/{name}[?force=true]
```
:::

Stop and start are one-click actions on the database detail page's header
(no confirmation, since neither is destructive: the data volume and
desired state are untouched either way). Delete is a confirm dialog on the
dashboard. It stops and removes the container and the database record but
**keeps the data volume** (`db-<name>-data`) and every backup in your storage
destination. If apps still connect to the database it is refused with a `409`
naming them; the dashboard then offers "Delete anyway" and the CLI takes
`--force`.

#### Creating a database with the name of a deleted one

Because the data volume survives a delete, a new database with the same name
would find the old data. The control plane no longer does that silently: the
create is refused with a `409` (`code: "existing_volume"`, listing the volumes
and their sizes) until you choose what to do with the old data:

| Choice | API field | CLI | Dashboard | Effect |
| --- | --- | --- | --- | --- |
| Reuse | `"existing_volume": "reuse"` | `--existing-volume reuse` | "Reuse old data" | The new database starts on the old data. Use the same engine and a compatible version. |
| Discard | `"existing_volume": "discard"` | `--existing-volume discard` | "Discard and start empty" | The old data, certificate and WAL archive volumes are deleted, then the database starts empty. Refused while a container still mounts one of them. |

```bash
levelrail-cli databases create --name main --engine postgres --version 16 --existing-volume discard
```

Verified live: delete a Postgres database holding a table, create it again with
no choice (refused), then with `discard` (the table is gone). The check covers
the control plane's own node only; a volume left on a remote node is not
inspected, so remove that one yourself. Restore-as-new into a name that has a
leftover volume is refused the same way.

### Changing the version

```bash
levelrail-cli databases set-version <name> 16.4
```

`PUT /api/v1/databases/{name}/version` (dashboard: "Change" next to the
version on the Overview tab) swaps the image tag. The container is stopped
and recreated over the same data volume, so take a backup first. Only minor
and patch changes are accepted for Postgres, MySQL, MariaDB, MongoDB and
ClickHouse: a major change would corrupt the data directory and is refused
with a `409`. Redis, KeyDB and Dragonfly may move to a newer major but never
back. For a Postgres major upgrade use the guarded flow below; for the other
engines, back up, then restore into a new database on the new version with
`backups restore-as-new`.

Postgres 18 and newer images moved their default data directory outside the
path this platform mounts, which would lose data on any container recreate.
For version 18 and above the controller pins `PGDATA` back to the mounted path
so the data stays on the volume.

### Major version upgrade (Postgres)

```bash
levelrail-cli databases major-upgrade <name> --version 17 [--confirm NAME]
levelrail-cli databases major-upgrades <name>
levelrail-cli databases major-upgrade-rollback <name> <id> [--confirm NAME]
levelrail-cli databases major-upgrade-discard <name> <id>
```

The dashboard has a "Major version upgrade" card on a Postgres database's
Overview tab (type the database name to enable the button).

The database is **offline** while this runs. The data files are never converted
in place. The phases, in order:

1. **preflight**: free space in the data directory, and the target image must be
   pullable. A version that does not exist fails here, before any downtime.
2. **suspend**: the database stops, cleanly.
3. **snapshot**: the data volume is copied to `db-<name>-data-pre<major>-<id>`.
4. **dump**: a temporary container on the *old* version starts on that snapshot
   (so the dump is consistent, because the live database is stopped, and the
   snapshot is proven to start), and `pg_dump` plus a per-table row count are
   taken from it. A dump without the pg_dump completion marker is refused.
5. **wipe, upgrade, restore**: the live volume is emptied, the version is
   changed, the reconciler starts the new version, and the dump is replayed in a
   single transaction.
6. **verify**: the server reports the target major, and the row count of every
   public table equals the old cluster's.

A failure before the wipe unsuspends the database on the old version and
removes the snapshot: nothing changed. A failure after the wipe (restore error,
count mismatch, the new version not starting) **rolls back automatically**: the
snapshot is copied back, the old version is restored, and the attempt is marked
`rolled_back`. If a control plane restart interrupts an upgrade, it settles the
attempt on the next start the same way. If a rollback itself fails, the attempt
is marked `failed` with the snapshot volume's name and that volume is left
intact.

After a success the snapshot is kept so `major-upgrade-rollback` can undo the
upgrade (every write since the upgrade is lost) until you run
`major-upgrade-discard` to free the disk. The snapshot volume is protected from
the orphaned volume cleanup while it is recorded.

Verified live on Postgres 16 to 17 with 5000 rows, a JSON column and a view
(all present after, `version()` reports 17.x); a manual rollback (16.x again, a
row written after the upgrade is gone); and a forced failure (a table owned by a
second role, which a single-database dump cannot recreate), which rolled back
automatically with the data intact.

Honest limits:

- Roles other than the database's own user are not part of a single-database
  dump. Objects owned by another role make the restore fail, which rolls back
  safely, but the upgrade cannot proceed until that ownership is changed.
- Only the `public` schema's tables are counted in verification, matching what
  restore recreates.
- Refused while point-in-time restore is enabled (its base backups and WAL
  cannot be replayed on another major version): disable it, upgrade, enable it
  again and take a new base backup. Downgrades across a major are refused.
- Needs free disk for the snapshot (the data volume's size again) on the Docker
  host plus the dump file in the data directory. A copy that runs out of disk
  fails before the wipe and changes nothing.
- Apps connected to the database see downtime for the whole run.
- Each wait (a container stopping, starting, accepting connections) is bounded by `APP_MAJOR_UPGRADE_WAIT`, a Go duration, default 5 minutes; a large cluster needing longer crash recovery should raise it.
- Postgres only. The other engines keep the backup and restore-as-new route.

## The backup story

Every backup, restore, and verify action for a database routes through a
`backup_targets` row: a connected S3-compatible bucket (AWS, Cloudflare
R2, or a custom endpoint) with its access key stored through the same
envelope-encrypted secrets path as any other credential. Create one from
**Settings -> Backup targets** or `levelrail-cli backup-targets create`
before any of what follows will work. Every backup/restore/verify
endpoint returns `501` if the control plane has no master key configured
at all (`internal/secrets`'s envelope encryption needs one to exist).

```mermaid
flowchart TD
  A[Backup target configured] --> B{Backup type}
  B -->|Manual trigger| C[Trigger immediately]
  B -->|Scheduled| D[Cron evaluates schedule]
  C --> E[Dump and upload]
  D --> E
  E --> F[Auto-verify scheduled only]
  F --> G{Verification passed?}
  G -->|Yes| H[Backup succeeded]
  G -->|No| I[Failed auto-verification]
  H --> J{Restore needed?}
  J -->|Destructive| K[Restore in place<br/>overwrites live data]
  J -->|Non-destructive| L[Restore as new<br/>creates fresh database]
  K --> M[Done]
  L --> M
```

### Manual trigger

```bash
levelrail-cli backups trigger <database> --target <backup-target-id>
```

`POST /api/v1/databases/{name}/backups` records the attempt and starts the
real dump-and-upload in the background, returning `202 Accepted`
immediately, not once the work finishes: the handler deliberately runs
`RunBackup` against `context.Background()`, not the request's own context,
because the request context is cancelled the instant the handler returns.
Poll `backups list` (or `GET .../backups`) to see whether it actually
succeeded.

```bash
$ levelrail-cli backups trigger main --target bkt_ax7f2j1kd
backup "bkh_p93kd7z1q" for database "main" started; check "levelrail-cli backups list main" for status

$ levelrail-cli backups list main
ID              TARGET         STATUS     SIZE     STARTED               FINISHED
bkh_p93kd7z1q   bkt_ax7f2j1kd  succeeded  2148291  2026-09-12T03:00:01Z  2026-09-12T03:00:14Z
```

Dashboard: the target picker and "Back up now" button in the **Backups**
card on a database's Overview tab
(`web/src/components/BackupsSection.tsx`).

### Scheduled backups

```bash
levelrail-cli backups schedule set <database> --target <id> --cron "0 3 * * *" [--retain N] [--retain-days N]
levelrail-cli backups schedule clear <database>
```

`PUT /api/v1/databases/{name}/backup-schedule` persists a target, a standard 5-field cron expression, and retention settings.

::: details Retention knobs (both independent, `0` means no limit)
- `retain`: Keep the last N successful backups
- `retain_days`: Delete anything older than N days
:::

**Validation and execution**

The cron expression is validated synchronously against `cronexpr.Parse`. A typo is a `400` at set-time, not a silently skipped tick. `internal/backup.Scheduler` evaluates every configured schedule on its own tick and runs the backup. Nothing else needs to be running for this to fire.

**Auto-verification**

Every scheduled backup that succeeds is automatically re-verified right after (`internal/backup.Scheduler`'s `Verifier`, wired in by default in `cmd/levelrail/main.go`). The verification badge shows "Auto-verified" or "Failed auto-verification" with `checked_by: "scheduler"`, distinguishing it from manual backups. Manual backups get no automatic follow-up; verify those yourself.

**Dashboard**

Use the schedule form at the top of the **Backups** card (`web/src/components/BackupScheduleForm.tsx`).

### Restore (destructive, in place)

```bash
levelrail-cli backups restore <database> --backup <backup-history-id> [--confirm <database-name>]
```

::: warning
`POST /api/v1/databases/{name}/restore` is the single most destructive endpoint in the API. It overwrites the target database's live data in place with no undo short of restoring from a different backup. It is gated at the `root` ability tier.
:::

Both the CLI and the dashboard require typing the database's exact name to confirm before the request is sent.

- **CLI**: Pass `--confirm <name>` to skip the interactive prompt. A script without both `--confirm` and a terminal attached is refused.
- **Dashboard**: The restore dialog (`RestoreBackupDialog.tsx`) keeps its button disabled until the text matches exactly.

Only a succeeded backup can be named as the restore source. The server returns `409` if you point it at a running or failed attempt.

```bash
$ levelrail-cli backups restore main --backup bkh_p93kd7z1q --confirm main
restore "rsh_k2n8fq31z" of database "main" from backup "bkh_p93kd7z1q" started; check "levelrail-cli backups list main" for status
```

### Restore-as-new / clone-restore (non-destructive)

```bash
levelrail-cli backups restore-as-new <database> --backup <id> --new-name <name> [--version V] [--project ID]
```

`POST /api/v1/databases/{name}/restore-as-new` creates a brand-new database and restores the backup into it, never touching the source database's live data. This is the safe way to test a migration or stand up a staging copy.

The new database inherits the source's engine always, and its version unless you override `--version`.

This endpoint is gated at `write:sensitive`, not `root`, because the worst case is an extra database you can delete like any other (unlike destructive in-place restore).

```bash
$ levelrail-cli backups restore-as-new main --backup bkh_p93kd7z1q --new-name main-staging
clone-restore "clr_h4t9wpq2m" of database "main" from backup "bkh_p93kd7z1q" into new database "main-staging" started; check "levelrail-cli databases get main-staging" for status
```

**Dashboard**

"Restore" and "Restore as new" buttons sit side by side on every succeeded row in the backup history table (`RestoreBackupDialog.tsx`, `CloneRestoreDialog.tsx`). These are separate buttons, not a mode toggle, so the safe action never looks as dangerous as the destructive one.

Past attempts show in their own history tables on the same card (`RestoreHistoryTable.tsx`, `CloneRestoreHistoryTable.tsx`).

**CLI**

List past attempts with `levelrail backups clone-restores <database>` (`GET /api/v1/databases/{name}/clone-restores`).

### Point-in-time restore (PITR): Postgres only

Every restore covered above is snapshot-based: it puts the database back
exactly as it was at the moment a specific backup (a `pg_dump`/`mysqldump`
logical dump) was taken, nothing in between. Point-in-time restore is
different: it can put a Postgres database back to *any exact timestamp*,
down to the second, not just to whenever a backup happened to run.

::: warning Only recoverable from when you enable it, forward
This is the single most important thing to understand before turning
PITR on: **it is never retroactive.** The moment you run `pitr enable`,
Postgres starts continuously archiving its write-ahead log (WAL). Only
timestamps from that moment onward are ever recoverable by point-in-time
restore. Nothing from before you enabled it can be restored this way,
only through an ordinary backup if one happened to exist from that
period. Enable it as early as you reasonably can, not after the fact.
:::

**How it works**

1. `pitr enable <database>` turns on continuous WAL archiving
   (`wal_level=replica`, `archive_mode=on`) going forward. Postgres only;
   no other engine implements this today.
2. Take a **physical base backup** (`pg_basebackup`, not a logical dump)
   with `pitr base-backups trigger`. This is the "floor" a restore
   replays WAL forward from. Take one periodically, the same way you'd
   schedule an ordinary backup: the more recent your latest base backup,
   the less WAL a restore has to replay to reach a given timestamp.
3. `pitr status <database>` reports the currently recoverable window:
   the oldest succeeded base backup's own timestamp on one end, and the
   latest instant WAL archiving has *provably* reached on the other
   (forced fresh, not a stale cached value, every time you ask).
4. `pitr restore <database> --base-backup ID --target-time RFC3339`
   restores to that exact timestamp: extracts the named base backup,
   replays archived WAL forward, and stops at the first transaction
   commit after `--target-time`.

```mermaid
flowchart LR
  A[pitr enable] --> B[WAL archives continuously]
  B --> C[pitr base-backups trigger]
  C --> D[pitr status<br/>shows recoverable window]
  D --> E[pitr restore --target-time]
  E --> F[Database restored to<br/>that exact second]
```

**Why the database goes offline during a restore**

Unlike an ordinary in-place restore (which pipes a dump into `psql`
against a still-running container), a PITR restore needs Postgres's own
data directory replaced out from under it, which cannot happen while the
server process is running against it. The control plane handles this for
you: it stops the container, wipes and repopulates its data volume from
the base backup, and lets the reconciler bring it back up once recovery
is configured. This is why a PITR restore takes noticeably longer than
an in-place logical restore, proportional to how much WAL there is to
replay, not to the database's total size.

**A target timestamp outside the recoverable window is rejected before
anything is touched** (`409`), the same synchronous-validation-first
discipline the ordinary restore endpoint already follows: a request
naming a timestamp before your oldest base backup, or after what's
actually been archived, never reaches the live database at all.

**Retention and disk use**

Archived WAL is first copied by Postgres into the database's `wal-archive`
Docker volume on the same host, and the control plane then **ships it to the
backup target** of the newest base backup, under `<database>/wal/` next to the
base backups. Shipping runs every minute (`APP_PITR_WAL_SHIP_INTERVAL`, a Go
duration, `0` disables it) and straight after every base backup, which first
forces the current segment out. A segment still being written is never shipped
(only files of full segment size are), and an object already in the bucket at
the same size is skipped. `pitr status` shows when WAL was last shipped and the
last error, in the CLI and on the dashboard card. A point-in-time restore pulls
the shipped WAL back into the archive volume before it touches the database,
overwriting same-named local files, so it works after the database host's disk
is lost.

Old data is pruned automatically: after every successful base backup the control
plane keeps the newest `APP_PITR_BASE_BACKUP_KEEP` (default 3) base backups,
deletes older ones and their bucket objects, and removes archived WAL older than
the oldest base backup that remains, both on the host and in the bucket, since a
restore can never start earlier. A base backup that a PITR restore record
references is kept. When a database has a backup schedule, each scheduled backup
of a PITR-enabled database is followed by a base backup, so there is no second
schedule to set up.

Honest limits of the remote copy: the data loss window after a disk failure is
up to one shipping interval of WAL (plus a segment Postgres has not archived
yet); shipping covers databases on the control plane's own node only; an object
that exists at the same size is never re-uploaded, so after a total host loss a
re-initialised cluster reuses segment names and the bucket keeps the original
lineage (which is what a restore wants). If the bucket is unreachable, shipping
fails visibly in `pitr status` and local WAL keeps accumulating on the host.

Verified live on Postgres 16 against a real S3 server: enable, base backup,
insert rows, restore to a timestamp between two inserts returned exactly the rows
from before it. Also verified from the remote copy alone: with the database
stopped and both its data volume and its `wal-archive` volume deleted, a restore
to the same timestamp returned exactly the rows from before it.

**CLI**

```bash
levelrail-cli pitr enable <database>
levelrail-cli pitr status <database>
levelrail-cli pitr base-backups trigger <database> --target ID
levelrail-cli pitr base-backups list <database>
levelrail-cli pitr restore <database> --base-backup ID --target-time RFC3339 [--confirm NAME]
levelrail-cli pitr disable <database>
```

```bash
$ levelrail-cli pitr enable main
point-in-time restore enabled for database "main"; take a base backup with "levelrail-cli pitr base-backups trigger main --target ID"

$ levelrail-cli pitr base-backups trigger main --target bkt_9f3ma
base backup "bbh_h2n8fq31z" for database "main" started; check "levelrail-cli pitr base-backups list main" for status

$ levelrail-cli pitr status main
enabled since 2026-09-20T00:00:00Z
recoverable window: 2026-09-20T00:00:00Z to 2026-09-22T14:32:07Z

$ levelrail-cli pitr restore main --base-backup bbh_h2n8fq31z --target-time 2026-09-21T09:00:00Z --confirm main
point-in-time restore "pitr_k2n8fq31z" of database "main" to "2026-09-21T09:00:00Z" started; check "levelrail-cli pitr status main" for the database's own condition once it finishes
```

Same "type the database's exact name to confirm" gate `backups restore`
already uses (`--confirm`, or an interactive prompt if you leave it off):
this is exactly as destructive as an ordinary in-place restore, gated at
the same `root` ability tier.

**Dashboard**

The database's Overview page has its own "Point-in-time restore" card,
separate from the ordinary Backups card: an Enable/Disable toggle, the
current recoverable window, base backup history with a manual trigger,
and a "Restore to timestamp" dialog with a datetime picker bounded to
the actual recoverable window (anything outside it can't even be typed
in).

List past point-in-time restore attempts with
`levelrail pitr restores <database>` (`GET /api/v1/databases/{name}/pitr-restores`).
Ordinary restore attempts are listed with `levelrail backups restores <database>`.

### ClickHouse, KeyDB and Dragonfly: what was verified

Run against real containers (ClickHouse 24.8, `eqalpha/keydb`, Dragonfly 1.27) with
hyphenated database names:

| Engine | Create | Backup | Verify | Restore as new |
| --- | --- | --- | --- | --- |
| ClickHouse | works (see below) | DDL plus rows as SQL | passed | rows identical, including a value with a quote and a comma |
| KeyDB | works | RDB snapshot | passed | keys identical |
| Dragonfly | works | RDB snapshot | passed | keys identical |

- **ClickHouse and hyphens.** The image's entrypoint puts `CLICKHOUSE_DB` and
  `CLICKHOUSE_USER` into unquoted SQL, so a database named `web-db` failed to
  create its database and user and came up with only the `default` one. The
  controller now passes `web_db` (hyphens and dots become underscores) for both,
  and the dump strips the source database's name from each table's DDL, because
  otherwise restoring into a database with a different name (restore-as-new)
  tried to create the tables in the source's name and failed. Backups taken
  before this fix restore into a database of the same ClickHouse identifier only.
- **In-place restore** (destructive) was also run for all three: ClickHouse rows
  and the KeyDB and Dragonfly key sets came back exactly as in the backup, and
  keys written after the backup were gone.
- **Dragonfly and small hosts.** Dragonfly starts one io thread per CPU and
  refuses to start when the threads times 256MiB exceed the memory available
  (`There are 14 threads, so 3.50GiB are required. Exiting...`), which a
  many-core host with little RAM hits, and which restart loops forever. Set
  `APP_DRAGONFLY_PROACTOR_THREADS` (for example `2`) on the control plane to cap
  it for Dragonfly databases created or recreated afterwards. An existing
  container keeps the command it was created with: remove it so the reconciler
  recreates it (the data volume is kept).
- **TLS** is Postgres and Redis only (see above). KeyDB, Dragonfly and ClickHouse
  connections are plaintext inside the Docker network.
- A failed restore-as-new leaves the new database in place with whatever state
  the failure left; delete it and retry.

### Restore safety checks

A Postgres restore replays the dump in one transaction with `ON_ERROR_STOP`: a
statement that fails rolls everything back and the restore reports failure, so a
damaged or partial dump leaves the existing data untouched. (It used to carry on
past errors and report success after dropping the schema.) Verified against a
real container with a deliberately broken dump.

Before any restore (in place or restore-as-new) the stored object is hashed in
a first pass and compared with the checksum recorded at backup time. A
corrupted object is refused with `object is corrupted` and the database is
not touched. Restore-as-new, and in-place restore into a freshly created
container, also wait until the server accepts TCP connections, so a restore
no longer races a database that is still initialising. Backups taken before
checksums were recorded skip the first pass.

Disk space: a manual backup is refused with `507` when the control plane's
data directory has less than `APP_MIN_BACKUP_DISK_MB` free (default 256),
because the backup history row itself is written there.

### Backup verification: re-download and re-hash

```bash
levelrail-cli backups verify <database> --backup <backup-history-id>
levelrail-cli backups verifications <database> --backup <backup-history-id>
```

`POST /api/v1/databases/{name}/backups/{historyId}/verify` re-downloads the backup's stored object from the bucket and checks it for corruption.

::: details Checks performed
- Checksum match
- Size match
- Lightweight structural check (`internal/backup.VerifyRunner`): the RDB header for Redis-family engines, and the completion trailer `pg_dump`, `mysqldump` and `mariadb-dump` write last, so a truncated dump fails

Verified live: flipping one byte of a stored Postgres dump makes the next verification fail with `CHECKSUM FAIL`.

The verification deliberately never attempts a live restore against a running database. That risk is out of scope for an automated check by design.
:::

**Status**

Like trigger and restore, the endpoint returns `202` immediately. The real work happens in the background. Use `backups verifications` (or the badge on the dashboard) to see results.

```bash
$ levelrail-cli backups verify main --backup bkh_p93kd7z1q
verification "bkv_9wq2ktz4h" of backup "bkh_p93kd7z1q" started; check "levelrail-cli backups verifications main --backup bkh_p93kd7z1q" for status

$ levelrail-cli backups verifications main --backup bkh_p93kd7z1q
[{"id":"bkv_9wq2ktz4h","backup_history_id":"bkh_p93kd7z1q","status":"passed","checksum_match":true,"size_match":true,"format_valid":true,"downloaded_bytes":2148291,"checked_by":"gagan","started_at":"2026-09-12T09:14:02Z","finished_at":"2026-09-12T09:14:05Z"}]
```

Dashboard: the verification badge and "Verify" button in the Verification
column of the backup history table
(`web/src/components/BackupVerificationBadge.tsx`).

### Downloading a raw backup

`GET /api/v1/databases/{name}/backups/{historyId}/download` streams the backup's stored object straight through, unbuffered. Large dumps are never held whole in memory, so you can keep a copy outside the platform entirely.

This endpoint is gated at `read:sensitive` (one tier above the metadata-only `read` tier that lists history) because the response body is potentially an entire production database's contents.

**Dashboard and CLI**

- Dashboard: Click the **Download** button next to Restore in the backup history table (auth rides the session cookie).
- CLI: No `backups download` subcommand exists today. Use `GET /api/v1/databases/{name}/backups/{historyId}/download` directly with your own bearer token for scripting.

## Slow query log viewer

Slow query logs capture individual statements that exceed a performance threshold, letting you find expensive queries without waiting for a production incident to surface them.

**Supported engines:** Postgres and MySQL only. Redis has no log-based slow query record (SLOWLOG is a live-server command, not stored in logs). MongoDB and other engines return an error when queried.

### How it works

**Postgres** reads slow statements from its own container log stream. Every Postgres container is started with `log_min_duration_statement=1000` by default (1 second), so any statement exceeding 1000ms is logged automatically. This threshold is built-in and not currently configurable per database.

**MySQL** reads the slow query log directly from the running container's file system (`/var/log/mysql/slow.log`), since MySQL's FILE log sink does not reliably open `/dev/stderr` from inside a container. The platform automatically configures MySQL with `long_query_time=1` and `slow_query_log=ON` at creation time.

**Query results** are sorted by duration descending and paginated by default at 100 entries per page (max 500).

### Viewing slow queries

**CLI**

```bash
levelrail-cli databases set-version <name> <version>
levelrail-cli databases major-upgrade <name> --version V [--confirm NAME]
levelrail-cli databases major-upgrades <name>
levelrail-cli databases major-upgrade-rollback <name> <id> [--confirm NAME]
levelrail-cli databases major-upgrade-discard <name> <id>
levelrail-cli databases slow-queries <name> [flags]
```

Flags:
- `--since DURATION` - how far back to search (default: 1h, e.g. "1h", "30m")
- `--from RFC3339` - start of search window (overrides `--since`)
- `--to RFC3339` - end of search window (default: now)
- `--limit N` - max entries to return (default: 100, max 500)
- `--offset N` - skip this many entries before returning results
- `--json` - output JSON array (default: human-readable table)

Example:

```bash
$ levelrail-cli databases slow-queries main --since 1h
Query                                              Duration (ms)  Rows Examined
SELECT * FROM large_table WHERE ...               2345.67        1500000
SELECT COUNT(*) FROM users WHERE ...              1523.21        2000000
INSERT INTO audit_log SELECT ...                  1100.45        0
```

**Dashboard**

Database Overview page has a **Slow Queries** tab with:
- A time-range picker (default: last 1 hour)
- A sortable table of slow queries sorted by duration descending
- Query text, duration in milliseconds, and rows examined (MySQL only)
- Live search is available when you query historical periods

**API**

`GET /api/v1/databases/{name}/slow-queries` returns:

```json
{
  "entries": [
    {
      "timestamp": "2026-09-22T14:32:07Z",
      "duration_ms": 2345.67,
      "query": "SELECT * FROM users WHERE...",
      "rows_examined": 1500000
    }
  ],
  "total": 342
}
```

Query parameters:
- `from` (RFC3339) - start of search window
- `to` (RFC3339) - end of search window
- `limit` - max entries (default 100, max 500)
- `offset` - pagination offset

::: details No slow queries in the results?
- **For Postgres:** Make sure the statement duration actually exceeded 1000ms. Very fast queries won't appear no matter how long your search window is.
- **For MySQL:** The slow query file exists on disk only after at least one slow statement has executed. If the database is brand new or has never had a slow query, the file doesn't exist yet and an empty result is normal.
- **Check the database is actually running:** If the container is stopped or crashed, there are no new slow query entries to log.
:::

## API reference

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/database-engines` | `read` |
| `GET` | `/api/v1/databases` | `read` |
| `POST` | `/api/v1/databases` | `write` |
| `GET` | `/api/v1/databases/{name}` | `read` |
| `DELETE` | `/api/v1/databases/{name}` | `write` |
| `GET` | `/api/v1/databases/{name}/status` | `read` |
| `GET` | `/api/v1/databases/{name}/metrics` | `read` |
| `GET` | `/api/v1/databases/{name}/logs` | `read` |
| `GET` | `/api/v1/databases/{name}/logs/stream` | `read` |
| `GET` | `/api/v1/databases/{name}/slow-queries` | `read` |
| `GET` | `/api/v1/databases/{name}/resource-recommendation` | `read` |
| `PUT` | `/api/v1/databases/{name}/node` | `root` |
| `PUT` | `/api/v1/databases/{name}/project` | `write` |
| `PUT` | `/api/v1/databases/{name}/resources` | `write` |
| `PUT` | `/api/v1/databases/{name}/public-access` | `write:sensitive` |
| `DELETE` | `/api/v1/databases/{name}/public-access` | `write:sensitive` |
| `POST` | `/api/v1/databases/{name}/stop` | `write:sensitive` |
| `POST` | `/api/v1/databases/{name}/start` | `write:sensitive` |
| `POST` | `/api/v1/databases/{name}/backups` | `write:sensitive` |
| `GET` | `/api/v1/databases/{name}/backups` | `read` |
| `GET` | `/api/v1/databases/{name}/backups/{historyId}/download` | `read:sensitive` |
| `POST` | `/api/v1/databases/{name}/backups/{historyId}/verify` | `write:sensitive` |
| `GET` | `/api/v1/databases/{name}/backups/{historyId}/verifications` | `read` |
| `PUT` | `/api/v1/databases/{name}/backup-schedule` | `write:sensitive` |
| `DELETE` | `/api/v1/databases/{name}/backup-schedule` | `write:sensitive` |
| `POST` | `/api/v1/databases/{name}/restore` | `root` |
| `GET` | `/api/v1/databases/{name}/restores` | `read` |
| `POST` | `/api/v1/databases/{name}/restore-as-new` | `write:sensitive` |
| `GET` | `/api/v1/databases/{name}/clone-restores` | `read` |
| `POST` | `/api/v1/databases/{name}/pitr` | `write:sensitive` |
| `DELETE` | `/api/v1/databases/{name}/pitr` | `write:sensitive` |
| `GET` | `/api/v1/databases/{name}/pitr` | `read` |
| `POST` | `/api/v1/databases/{name}/base-backups` | `write:sensitive` |
| `GET` | `/api/v1/databases/{name}/base-backups` | `read` |
| `POST` | `/api/v1/databases/{name}/pitr-restore` | `root` |
| `GET` | `/api/v1/databases/{name}/pitr-restores` | `read` |
| `POST` | `/api/v1/databases/{name}/major-upgrade` | `root` |
| `GET` | `/api/v1/databases/{name}/major-upgrades` | `read` |
| `POST` | `/api/v1/databases/{name}/major-upgrades/{id}/rollback` | `root` |
| `DELETE` | `/api/v1/databases/{name}/major-upgrades/{id}/snapshot` | `root` |
| `PUT` | `/api/v1/apps/{name}/database` | `write` |
| `DELETE` | `/api/v1/apps/{name}/database` | `write` |
| `GET`/`POST`/`PUT`/`DELETE` | `/api/v1/backup-targets` (+ `/{id}`, `/{id}/test`) | `read` (GET) / `write:sensitive` (everything else) |

## CLI

```bash
levelrail-cli databases create --name NAME --engine ENGINE --version VERSION [--node-id ID] [--existing-volume reuse|discard]
levelrail-cli databases create --interactive   # also prompts for resource limits, public access, backup schedule
levelrail-cli databases list
levelrail-cli databases get <name>
levelrail-cli databases delete <name>
levelrail-cli databases stop <name>
levelrail-cli databases start <name>
levelrail-cli databases resource-recommendation <name>
levelrail-cli databases set-resources <name> [--memory 512Mi] [--cpu 0.5] [--swap-memory SIZE] [--cpuset-cpus RANGE]
levelrail-cli databases metrics <name> --metric NAME [flags]
levelrail-cli databases set-project <name> <project-id>
levelrail-cli databases clear-project <name>
levelrail-cli databases public-access set <name> [--port N] [--bind-address ADDR]
levelrail-cli databases public-access clear <name>
levelrail-cli databases slow-queries <name> [--since DURATION] [--from RFC3339] [--to RFC3339] [--limit N] [--offset N]

levelrail-cli backups list <database> [--limit N] [--before TIMESTAMP]
levelrail-cli backups trigger <database> --target ID
levelrail-cli backups restore <database> --backup ID [--confirm NAME]
levelrail-cli backups restore-as-new <database> --backup ID --new-name NAME [--version V] [--project ID]
levelrail-cli backups schedule set <database> --target ID --cron EXPR [--retain N] [--retain-days N]
levelrail-cli backups schedule clear <database>
levelrail-cli backups verify <database> --backup ID
levelrail-cli backups verifications <database> --backup ID

levelrail-cli pitr enable <database>
levelrail-cli pitr disable <database>
levelrail-cli pitr status <database>
levelrail-cli pitr base-backups list <database>
levelrail-cli pitr base-backups trigger <database> --target ID
levelrail-cli pitr restore <database> --base-backup ID --target-time RFC3339 [--confirm NAME]

levelrail-cli backup-targets create --name NAME --provider PROVIDER --bucket BUCKET --access-key-id ID --secret-access-key SECRET [--endpoint URL] [--region REGION]
levelrail-cli backup-targets list
levelrail-cli backup-targets get <id>
levelrail-cli backup-targets delete <id>
levelrail-cli backup-targets test <id>
```

Backup scheduling has no standalone `databases` subcommand: it's reachable
through `databases create --interactive`'s wizard at creation time, or by
calling its dedicated API route directly. Resource limits and public
access both have their own subcommand (`databases set-resources`,
`databases public-access set`/`clear`, above), the same shape
`set-project`/`clear-project` already establish for a different
per-database setting. The dashboard's own creation dialog is deliberately
narrower than the CLI's interactive wizard too: it collects only
name/engine/version/node up front (`CreateDatabaseFields.tsx`), the same
fields `databases create` takes without `--interactive`; resource limits,
public access, and a backup schedule are all configured afterward from the
database's own Overview and Resources tabs once it exists.

::: details Not built yet (deliberate follow-ups)

- **No CLI download command**
  `GET .../backups/{historyId}/download` works from the dashboard and from any HTTP client with a bearer token. There is no `backups download` subcommand.

- **Delete does not stop the running container**
  `DELETE /api/v1/databases/{name}` removes desired state only, the same gap as `DELETE /api/v1/apps/{name}`. A container can outlive its desired-state row until something else tears it down.

- **No secret deletion on a deleted backup target**
  `internal/secrets` has no revoke operation. A backup target's stored access key and secret remain in the secrets store after `DELETE /api/v1/backup-targets/{id}`, unreferenced but not erased at rest.

- **TLS is Postgres and Redis only, with no operator toggle**
  MySQL/MariaDB lack a driver-agnostic "encrypt without verifying" URI option. MongoDB's equivalent exists but isn't wired up. KeyDB takes Redis's flags unchanged but is not wired; Dragonfly needs authentication enabled first. There is also no way to opt out of TLS for Postgres/Redis if you wanted to.

- **No scheduler catch-up after downtime**
  If the control plane is down when a scheduled backup should fire, that run is missed, not queued or caught up on restart. Deliberately deferred because catch-up needs design work (how many missed runs to replay, how to avoid a thundering herd after a long outage).

- **No automatic scheduling for PITR base backups**
  Ordinary logical backups can run on a cron (`backups schedule set`). Physical base backups for point-in-time restore are manual-trigger only today (`pitr base-backups trigger`), dashboard button or CLI/API call. The longer you go without taking a fresh one, the more WAL a restore has to replay to reach a recent timestamp. Take one periodically yourself until this gets its own schedule.

:::

## See also

- [Identity and access](identity-and-access.md): Backup target credentials are stored through envelope encryption, gated at `write:sensitive` ability tier.
- [App spec reference](app-spec-reference.md): Bind addresses and exposure rules for database public access.
- [Deploying apps](deploying-apps.md): How to attach managed databases to applications.
