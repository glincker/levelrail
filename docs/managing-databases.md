# Managing databases

A managed database is a Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB,
Dragonfly, or ClickHouse container the reconciler runs, backs up, and
tracks the same way it tracks an app, except there is no build step and no
domain to route. Packages: `internal/api/databases.go`,
`internal/reconcile/database`, `internal/backup`, `internal/store` (the
`database_engines.yaml` registry and the desired-state schema).

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

Creating a database always succeeds at the store layer, even for engines
that need a generated credential. `POST /api/v1/databases` writes the
desired state (name, engine, version) and returns immediately; the
reconciler is what actually starts the container on its next pass, and it
refuses to start a database whose credentials don't exist yet, reporting
that refusal as a real condition rather than pretending the engine isn't
supported. `GET /api/v1/databases/{name}/status` (or `databases get`,
which shows status inline) is how you find out whether it's actually
running, not just declared.

Placement follows the same rule as apps: omit `node_id` at creation and
simple spread scheduling picks a node (or the local node if only one
exists); pass one explicitly to override it. Moving an already-created
database to a different node is a separate call,
`PUT /api/v1/databases/{name}/node`, gated at the root ability tier because
node placement is fleet-level infrastructure, not ordinary config.

Stop and start don't touch desired state or the data volume at all: they
flip a `suspended` flag, and the reconciler removes or recreates the
container on its next pass against the exact same volume. Delete is
different and has a known gap worth knowing about: `DELETE
/api/v1/databases/{name}` removes the desired-state row but does not
itself stop or remove the running container, the same gap `DELETE
/api/v1/apps/{name}` has.

## TLS: on by default, for two engines, with no toggle

Postgres and Redis get TLS enabled automatically at creation time
(`internal/reconcile/database`'s `WithTLS`), with a self-signed
certificate generated once and persisted through the same secrets
mechanism a generated password uses. There is no operator switch for this,
on the dashboard or the CLI: `databaseResource.TLSEnabled` is a read-only,
computed field, true exactly when a TLS certificate has actually been
generated for that database.

The reason it's scoped to just those two engines: both expose an
"encrypt without verifying" mode entirely inside a standard connection
URI (`sslmode=require` for Postgres, `rediss://` for Redis) that mainstream
client libraries already honor with zero app-side code changes. MySQL and
MariaDB have no driver-agnostic URI knob for that; MongoDB's equivalent
option exists but isn't wired up yet; KeyDB and Dragonfly fork Redis's TLS
flags under names this codebase hasn't verified. The certificate is
self-signed and never distributed to a party that verifies its issuer (no
app ever checks it), so it's valid for ten years and rotation is a
deliberate future operator action, not something a short expiry forces.

An app that attaches to a TLS-enabled database (see below) automatically
gets the TLS-flavored connection string: `resolveDatabaseURL` in
`internal/reconcile/application` appends `?sslmode=require` for Postgres or
switches to `rediss://` (and the TLS-only port) for Redis. Nothing in
`app.yaml` or the attach request opts into this; it just reflects the
database's own state.

## Resource limits

Memory, CPU, swap, and a CPU pin (`cpuset`) are ordinary desired state, the
same `ServiceResources` shape an app's resources use, but there is no
general `PUT /databases/{name}` to fold them into (unlike apps), so they
get their own route: `PUT /api/v1/databases/{name}/resources`. Set to
`null`/omitted, limits clear. When the database has a container already
running, the new limits get pushed onto it live; if that's not possible
(no container yet, node unreachable), they apply the next time the
container restarts, and the response's `resources_applied_live` field
tells you which happened, exactly the pattern `resource_recommendation`
pairs with: `GET /api/v1/databases/{name}/resource-recommendation`
suggests memory/CPU numbers from the database's own historical usage, the
same recommendation feature apps have.

Dashboard: the database's own **Resources** tab
(`web/src/routes/databases/$name/resources.tsx`), with a recommendation
card above the limits editor.

## Public access: exposing a database port directly

By default every managed database is reachable only from inside the
platform's own Docker network, the same as any other backing service.
`PUT /api/v1/databases/{name}/public-access` binds it to a host port too,
which is the actual missing link for pointing pgAdmin, TablePlus, or
RedisInsight at a managed database from your own machine.

Leave `port` at `0` (or omit it) to auto-assign the next free port, or
request a specific one in the `1024-65535` range; ports the control plane's
own listeners already use (`80`, `443`, `8080`, `9443`) are rejected
outright rather than left to fail as an opaque Docker bind error later.
Requesting a different port while already public is a real state change,
not an edit-in-place: the dashboard's port field is accordingly only
editable before you enable access, not after.

`bind_address` picks which network interface that port binds to:
`private` (loopback only, the default when omitted), `public` (every
interface, an explicit opt-in), or a literal IP. Same shorthand and
resolution rules as an app service's own `bind_address`
(`internal/bindaddr.Resolve`); see
[app-spec-reference.md's Bind addresses and exposure](app-spec-reference.md#bind-addresses-and-exposure)
for the full table. A database already publicly accessible before this
field existed keeps that exposure (backfilled to `public`); re-enabling
public access afterward without an explicit `bind_address` picks up the
new `private` default instead.

`DELETE /api/v1/databases/{name}/public-access` reverts to internal-only;
the reconciler replaces the running container without the host port
binding on its next pass (a replace, not a plain restart).

Redis, KeyDB, and Dragonfly run passwordless by default in this platform,
so publishing their port means unauthenticated read/write access to the
whole dataset, a materially different risk than Postgres/MySQL's generated
passwords. The dashboard's public-access card gates the toggle behind an
explicit "I understand this database has no password" checkbox for that
engine family; Postgres/MySQL/MongoDB/MariaDB/ClickHouse get a plain,
non-blocking warning instead.

Dashboard: **Public access** card on the database's Overview tab
(`web/src/components/DatabasePublicAccessCard.tsx`). CLI:
`levelrail-cli databases public-access set <name> [--port N] [--bind-address ADDR]`
and `levelrail-cli databases public-access clear <name>`
(`cmd/levelrail-cli/databases_public_access.go`); also reachable through
`databases create --interactive`'s wizard (which calls the same PUT
endpoint as a create-time follow-up, always at the default bind address).

## Attaching a database to an app

An app that wants a database's connection value doesn't have to be
deployed from an `app.yaml` with a `{ from: "<database>.<field>" }` env
var: `PUT /api/v1/apps/{name}/database` attaches an already-created
database directly, and the reconciler injects the resolved value as a real
env var the next time that app's container is (re)created.

`database_name` is the only required field. `env_var` defaults to
`DATABASE_URL` and `field` defaults to `url` (a full connection string),
covering the common case with one call; the other resolvable fields are
`host`, `port`, `username`, `password`, and `database`, validated against
`database.SupportsField`'s engine-aware rules before anything is saved (a
400, not a later reconcile failure). `username`/`database` aren't
resolvable for Redis, KeyDB, or Dragonfly, since none of the three model a
username or a named database. `DELETE /api/v1/apps/{name}/database`
detaches; the next reconcile pass stops injecting the env var into freshly
created containers, it does not retroactively touch a container already
running.

Dashboard: the **Database** card on an app's own Overview page
(`web/src/components/DatabaseAttachmentCard.tsx`), with an "Env var / field
options" disclosure for anything past the URL default. This lives on the
app's page, not the database's, since the attachment is really the app's
own env var source.

## Stop, start, and delete: dashboard and CLI

```bash
levelrail-cli databases stop <name>
levelrail-cli databases start <name>
levelrail-cli databases delete <name>
```

Stop and start are one-click actions on the database detail page's header
(no confirmation, since neither is destructive: the data volume and
desired state are untouched either way). Delete is a confirm dialog on the
dashboard; the CLI has no `--confirm` flag for it today, matching
`handleDeleteDatabase`'s own scope, it removes desired state only.

## The backup story

Every backup, restore, and verify action for a database routes through a
`backup_targets` row: a connected S3-compatible bucket (AWS, Cloudflare
R2, or a custom endpoint) with its access key stored through the same
envelope-encrypted secrets path as any other credential. Create one from
**Settings -> Backup targets** or `levelrail-cli backup-targets create`
before any of what follows will work. Every backup/restore/verify
endpoint returns `501` if the control plane has no master key configured
at all (`internal/secrets`'s envelope encryption needs one to exist).

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

`PUT /api/v1/databases/{name}/backup-schedule` persists a target, a
standard 5-field cron expression, and two independent retention knobs:
`retain` (keep the last N successful backups) and `retain_days` (delete
anything older than N days), both `0` meaning no limit on that dimension.
The cron expression is validated synchronously against `cronexpr.Parse`,
so a typo is a `400` at set-time, not a silently skipped tick later.
`internal/backup.Scheduler` evaluates every configured schedule on its own
tick and runs the backup for you; nothing else needs to be running for
this to fire.

One behavior worth knowing that has no toggle anywhere: every scheduled
backup that succeeds is automatically re-verified right after
(`internal/backup.Scheduler`'s `Verifier`, wired in by default in
`cmd/levelrail/main.go`). Its verification badge shows "Auto-verified" (or
"Failed auto-verification") with `checked_by: "scheduler"`, distinguishing
it from one you triggered by hand. Manual backups get no such automatic
follow-up; verify those yourself (below).

Dashboard: the schedule form at the top of the same **Backups** card
(`web/src/components/BackupScheduleForm.tsx`).

### Restore (destructive, in place)

```bash
levelrail-cli backups restore <database> --backup <backup-history-id> [--confirm <database-name>]
```

`POST /api/v1/databases/{name}/restore` is, by this codebase's own
assessment, the single most destructive endpoint in the whole API: it
overwrites the target database's live data in place, with no undo short of
restoring again from a different backup. It's gated at the `root` ability
tier, one step above `write:sensitive`, the same tier as node management.

Both the CLI and the dashboard require typing the database's exact name to
confirm before the request is ever sent. On the CLI, pass `--confirm <name>`
to skip the interactive prompt (a script with neither `--confirm`
nor a terminal attached is refused, not silently let through); the
dashboard's restore dialog (`RestoreBackupDialog.tsx`) keeps its button
disabled until the typed text matches character for character. Only a
succeeded backup can be named as the restore source; the server checks
this before starting anything, returning `409` if you point it at a
running or failed attempt.

```bash
$ levelrail-cli backups restore main --backup bkh_p93kd7z1q --confirm main
restore "rsh_k2n8fq31z" of database "main" from backup "bkh_p93kd7z1q" started; check "levelrail-cli backups list main" for status
```

### Restore-as-new / clone-restore (non-destructive)

```bash
levelrail-cli backups restore-as-new <database> --backup <id> --new-name <name> [--version V] [--project ID]
```

`POST /api/v1/databases/{name}/restore-as-new` creates a brand-new
database (through the identical creation path `databases create` uses)
and restores the named backup into it, never touching the source
database's own live data. This is the standard way to test a migration
against real data or stand up a staging copy without any of the risk
`backups restore` carries, which is also why it's gated at
`write:sensitive`, not `root`: the worst case is an extra database you can
delete like any other. The new database inherits the source's engine
always, and its version unless you override `--version`.

```bash
$ levelrail-cli backups restore-as-new main --backup bkh_p93kd7z1q --new-name main-staging
clone-restore "clr_h4t9wpq2m" of database "main" from backup "bkh_p93kd7z1q" into new database "main-staging" started; check "levelrail-cli databases get main-staging" for status
```

Dashboard: "Restore" and "Restore as new" buttons sit side by side on
every succeeded row in the backup history table
(`RestoreBackupDialog.tsx`, `CloneRestoreDialog.tsx`), deliberately kept as
two separate buttons rather than a mode toggle on one, so the safe action
never looks as dangerous as the destructive one or vice versa. Past
attempts of both kinds show in their own history tables on the same card
(`RestoreHistoryTable.tsx`, `CloneRestoreHistoryTable.tsx`); the CLI has no
`clone-restores` list subcommand today, only the trigger, though `GET
/api/v1/databases/{name}/clone-restores` exists for scripting against
directly.

### Backup verification: re-download and re-hash

```bash
levelrail-cli backups verify <database> --backup <backup-history-id>
levelrail-cli backups verifications <database> --backup <backup-history-id>
```

`POST /api/v1/databases/{name}/backups/{historyId}/verify` re-downloads
the backup's stored object from the bucket and checks it for corruption:
checksum match, size match, and a lightweight structural check
(`internal/backup.VerifyRunner`). It deliberately never attempts a live
restore against a running database, that risk is out of scope for an
automated check by design. Like trigger and restore, it returns `202`
immediately and the real work happens in the background; `backups
verifications` (or the badge on the dashboard) is how you see whether it
passed.

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

`GET /api/v1/databases/{name}/backups/{historyId}/download` streams the
backup's own object straight through, unbuffered (a large dump is never
held whole in memory), so you can keep a copy outside the platform
entirely. Gated at `read:sensitive`, one tier above the metadata-only
`read` that lists history, since the response body here is potentially an
entire production database's contents, not just a size and a timestamp.
Dashboard: the **Download** button next to Restore in the backup history
table. There is no CLI subcommand for this today; it's a plain
browser-navigated download on the dashboard (auth rides the same session
cookie every same-origin request already uses), reachable by scripting
against the endpoint directly with your own bearer token otherwise.

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
| `PUT` | `/api/v1/apps/{name}/database` | `write` |
| `DELETE` | `/api/v1/apps/{name}/database` | `write` |
| `GET`/`POST`/`PUT`/`DELETE` | `/api/v1/backup-targets` (+ `/{id}`, `/{id}/test`) | `read` (GET) / `write:sensitive` (everything else) |

## CLI

```bash
levelrail-cli databases create --name NAME --engine ENGINE --version VERSION [--node-id ID]
levelrail-cli databases create --interactive   # also prompts for resource limits, public access, backup schedule
levelrail-cli databases list
levelrail-cli databases get <name>
levelrail-cli databases delete <name>
levelrail-cli databases stop <name>
levelrail-cli databases start <name>
levelrail-cli databases resource-recommendation <name>
levelrail-cli databases metrics <name> --metric NAME [flags]
levelrail-cli databases set-project <name> <project-id>
levelrail-cli databases clear-project <name>
levelrail-cli databases public-access set <name> [--port N] [--bind-address ADDR]
levelrail-cli databases public-access clear <name>

levelrail-cli backups list <database> [--limit N] [--before TIMESTAMP]
levelrail-cli backups trigger <database> --target ID
levelrail-cli backups restore <database> --backup ID [--confirm NAME]
levelrail-cli backups restore-as-new <database> --backup ID --new-name NAME [--version V] [--project ID]
levelrail-cli backups schedule set <database> --target ID --cron EXPR [--retain N] [--retain-days N]
levelrail-cli backups schedule clear <database>
levelrail-cli backups verify <database> --backup ID
levelrail-cli backups verifications <database> --backup ID

levelrail-cli backup-targets create --name NAME --provider PROVIDER --bucket BUCKET --access-key-id ID --secret-access-key SECRET [--endpoint URL] [--region REGION]
levelrail-cli backup-targets list
levelrail-cli backup-targets get <id>
levelrail-cli backup-targets delete <id>
levelrail-cli backup-targets test <id>
```

Resource limits and backup scheduling have no standalone `databases`
subcommand: they're reachable through `databases create --interactive`'s
wizard at creation time, or by calling their dedicated API routes
directly. Public access does have its own subcommand
(`databases public-access set`/`clear`, above), the same shape
`set-project`/`clear-project` already establish for a different
per-database setting. The dashboard's own creation dialog is deliberately
narrower than the CLI's interactive wizard too: it collects only
name/engine/version/node up front (`CreateDatabaseFields.tsx`), the same
fields `databases create` takes without `--interactive`; resource limits,
public access, and a backup schedule are all configured afterward from the
database's own Overview and Resources tabs once it exists.

## Not built yet (deliberate follow-ups)

- **No CLI subcommand for resource limits outside the creation wizard.**
  It has a real, working API route (`PUT .../resources`) and a dashboard
  control; there's no `databases set-resources` for scripting an existing
  database after the fact today. (Public access got its own subcommand,
  `databases public-access set`/`clear`, above.)
- **No CLI download command.** `GET .../backups/{historyId}/download`
  works from the dashboard (a plain authenticated browser navigation) and
  from any HTTP client with a bearer token; there's no `backups download`
  subcommand.
- **No CLI list command for clone-restore history.** `backups
  restore-as-new` triggers one; `GET .../clone-restores` exists to list
  past attempts, but only the dashboard's `CloneRestoreHistoryTable`
  reads it today.
- **Delete does not stop the running container.** `DELETE
  /api/v1/databases/{name}` removes desired state only, the same known gap
  `DELETE /api/v1/apps/{name}` carries; a container can outlive its own
  desired-state row until something else tears it down.
- **No secret deletion on a deleted backup target.** `internal/secrets`
  has no revoke operation, so a backup target's stored access key and
  secret remain in the secrets store after `DELETE
  /api/v1/backup-targets/{id}`, unreferenced but not erased at rest.
- **TLS is Postgres and Redis only, with no operator toggle at all.**
  MySQL/MariaDB have no driver-agnostic "encrypt without verifying" URI
  option to standardize on; MongoDB's equivalent exists but isn't wired
  up; KeyDB/Dragonfly's Redis-derived TLS flags haven't been verified
  against this codebase's assumptions. There's also no way to opt out of
  TLS for Postgres/Redis if you wanted to.
- **No scheduler catch-up after downtime.** If the control plane is down
  when a scheduled backup should have fired, that run is simply missed,
  not queued or caught up on restart. Deliberately deferred: getting
  catch-up right needs its own design (how many missed runs to replay, how
  to avoid a thundering herd after a long outage) that this feature's
  scope didn't ask for.
