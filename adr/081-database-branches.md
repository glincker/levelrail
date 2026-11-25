# ADR 081: Database branches for every managed engine

Status: Proposed

## Context

Users want the branch workflow a serverless Postgres provider made popular: fork a database in one dialog, work against the copy, throw it away or reset it, and get one per pull request without thinking about it. That provider gets instant, copy on write branches from its own storage engine. Levelrail runs plain upstream engine images on ordinary Linux disks, so the same dialog has to be backed by honest, per engine and per host mechanics.

What exists on `release/v1.5.11`, read in code:

- `POST /api/v1/databases/{name}/branches` (`internal/api/database_branch.go`, routed in `routes_platform_backup.go` at `AbilityWriteSensitive`) creates a new independent database from a succeeded backup (`backup_id`) or, for MySQL and MariaDB with PITR enabled, from a point in time (`at`, through `MySQLPITRRunner.RunPITRCloneRestore`). It goes through `createDesiredDatabase`, so a branch is an ordinary `desired_databases` row with its own container, volume (`db-<name>-data`) and generated credentials. The pooler is turned off on the new database. The only lineage kept is a `clone_restores` row (`migrations/0074_clone_restores.sql`): source name, new name, backup id, status. There is no parent pointer, expiry, protection, mode or size on the branch itself.
- CLI `databases branch create <source> <new> (--from-backup ID | --at TIME)` (`cmd/levelrail-cli/databases_branch.go`) and `databases clone` (sugar over restore-as-new). Web `components/databases/BranchDatabaseDialog.tsx`.
- Restore-as-new: `backup.CloneRestoreRunner` waits for the new database's reconciler to report Ready, then streams download and apply through the normal `Restorer`. Volume counterpart: `VolumeCloneRestoreRunner`.
- Dumps (`internal/backup/dump.go`) run the engine client inside the container: `pg_dump --no-owner --no-privileges`, `mysqldump` and `mariadb-dump` of the single app database, `mongodump --archive`, `redis-cli --rdb -` (KeyDB and Valkey alike, Dragonfly has its own path), ClickHouse and search engines through their own dumpers, Memcached none (`store.EngineSupportsBackups`).
- Postgres PITR exists (base backup plus WAL archive, `pitr_runner.go`, `pitr_restore.go`) but `PITRRestorer` restores in place into a stopped container's volume. MySQL family PITR can already land in a new database.
- Preview environments: `spec.Database.EphemeralInPreviews` provisions a full disposable instance per pull request (`preview_environments_databases.go`, table `preview_ephemeral_databases`), `SeedFrom` is `none` or `latest` (newest backup). `IsolatedInPreviews` gives a per preview role (Postgres) or ACL user (Redis) on the shared database instead.
- Disk: `internal/diskguard` refuses ops `build`, `pull`, `backup` at critical free space (`APP_DISK_LOW_FREE_MB`, `APP_DISK_CRITICAL_FREE_MB`, `APP_DISK_RESERVE_MB`, `APP_DISK_DOCKER_ROOT`) with a 507; the branch handler uses `backup.CheckWorkDirSpace` (`APP_MIN_BACKUP_DISK_MB`), which guards only the bookkeeping write.
- Database delete keeps the data volume on purpose (`databases.go`, `database_leftover_volume.go`), so a deleted database's bytes stay on disk until a same named create discards them.
- `internal/datamigrate` copies an external database into a managed one through a helper container with scrubbed errors and verification; its copier and sanitizer are the closest existing code to a streaming transform.
- Live tunables go through `internal/runtimesettings` (env seeds the default, the API, CLI and dashboard change it at runtime).
- No reflink, btrfs or zfs detection exists anywhere in `internal/`.

The gap is everything around the copy: lineage, modes, expiry, reset, protection, anonymization, size and activity, compare, and a branch per pull request that is a real branch of a named parent rather than an empty or backup seeded instance.

## Decision

A branch is an ordinary managed database plus one `database_branches` row that records its lineage and lifecycle. Branching never writes to the parent, and nothing about a branch is reachable (no network alias, no published port, no app link, no credentials reveal) until the branch reaches `ready`. One mechanism, three sources, four modes, two clone paths:

- Sources: `live` (a consistent dump of the parent now, new), `backup` (existing restore-as-new), `pitr` (a past point in time, existing for MySQL family, Postgres behind F08).
- Modes: `full` (data and schema), `pitr` (data and schema at a past time), `schema` (schema only), `anonymized` (data through masking rules).
- Clone paths: `dump` (slow path, every supported engine) and `snapshot` (fast path, only when the host proves it can and the mode is `full` from `live`).

The recommendation is to ship the slow path for all five engine families first, make the fast path a detected optimization that falls back silently and reports why, ship anonymization as a stream transform only (never as post restore updates), and keep promote as "detach from parent" rather than a name swap.

### Data model

Two migrations, numbered at the next free slot when the slice lands (currently after 3320):

- `NNNN_database_branches.sql`: table `database_branches`.
- `NNNN_database_masking_rules.sql`: tables `database_masking_rulesets` and `database_masking_rules`.

`database_branches`:

| Column | Meaning |
| --- | --- |
| id | text primary key |
| database_name | the branch's own `desired_databases.name`, unique |
| parent_name | parent database name (a root or another branch) |
| root_name | the root of the tree, for listing and quota |
| mode | `full`, `pitr`, `schema`, `anonymized` |
| source | `live`, `backup`, `pitr` |
| source_ref | backup id, or RFC3339 target for `pitr`, empty for `live` |
| clone_path | `dump` or `snapshot`, plus `clone_path_reason` text when the fast path was skipped |
| state, state_reason | lifecycle below |
| protected | integer 0 or 1 |
| expires_at, expiry_preset | empty means never; preset is the label the user picked |
| ruleset_id, ruleset_version | set only for `anonymized`, pins the exact rules applied |
| preview_environment_id | nullable, set for pull request branches |
| size_bytes, size_measured_at | volume usage, sampled |
| last_active_at | last observed client activity, sampled |
| branched_at | the parent's point the data reflects |
| created_by, created_at, updated_at | principal id and times |

Indexes on `(root_name)`, `(parent_name)`, `(state, expires_at)`, `(preview_environment_id)`. Deleting a parent that has children is refused; children must be deleted or promoted first. The existing `clone_restores` row is still written for `backup` and `pitr` sources so restore history stays one place.

`database_masking_rulesets`: id, database_name (the parent the rules belong to), version (incremented on every save, older versions kept read only), created_by, created_at. `database_masking_rules`: id, ruleset_id, target (`schema.table.column` for SQL, `collection.field.path` for MongoDB), kind, params (JSON, no secret values), deterministic flag, salt_ref (a key in `internal/secrets`, never the salt itself), position.

### Lifecycle states

`requested` then `cloning` then (`masking` for anonymized) then `credentials` (rotate every role, apply branch credentials) then `verifying` (row or key counts against the dump manifest) then `ready`. From `ready`: `resetting` back to `ready`, `promoting` to a standalone database (row kept with `state=promoted`, parent cleared), `expiring` and `deleting` to `deleted`. Any step can go to `failed` with `state_reason`; a failed branch is never reachable and is deleted by the expiry job after `branches.failed_retention`. State moves are compare and set on `(id, state)` so two operators or the job cannot race.

### Engine mapping

| Engine | full live | pitr | schema | anonymized | fast path | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| Postgres | yes, `pg_dump` (one repeatable read snapshot by design) | after F08 | `pg_dump --schema-only` | yes, COPY stream | yes | roles are not in a single database dump; branch gets its own owner role |
| MySQL | yes, `mysqldump --single-transaction --routines --triggers --events` | yes (exists) | `--no-data` | yes, INSERT stream | yes, InnoDB crash recovery | non InnoDB tables are not consistent under `--single-transaction`; the dialog warns when any exist |
| MariaDB | yes, `mariadb-dump --single-transaction ...` | yes (exists) | `--no-data` | yes | yes | same Aria and MyISAM warning |
| MongoDB | yes, `mongodump --archive --oplog` | no | collections, indexes, validators, no documents | yes, BSON stream | yes, WiredTiger journal | `--oplog` needs a replica set; a standalone parent falls back to a dump without it and the branch records `consistency=best_effort` |
| Redis, KeyDB, Valkey | yes, `--rdb -` (point in time by the engine) | no | no, there is no schema | no | yes | anonymization refused: keys and values have no declared structure to target safely |
| Dragonfly | yes, its own snapshot path | no | no | no | no | snapshot format differs, fast path not attempted |
| ClickHouse | no | no | no | no | no | backup exists but restore into a fresh server is not proven; refused with that reason |
| Meilisearch, Qdrant, Typesense | no | no | no | no | no | index data is rebuilt from a source of truth, branching adds little; refused |
| Memcached | no | no | no | no | no | no persistent state to branch |

The dialog reads availability from one capability endpoint, so a disabled mode always shows its reason.

### How the clone works

Slow path (`dump`): create the branch's desired database through `createDesiredDatabase` with network isolation on (no aliases, no published port, no links), wait for Ready as `CloneRestoreRunner` does, then stream parent dump to branch restore through a helper with no file on local disk. `backup` and `pitr` sources reuse the restore-as-new and MySQL PITR clone runners unchanged. `live` is new: it runs the engine's consistent dump command against the parent container and pipes it into the branch's `Restorer`. Schema mode is the same pipe with the schema only flags.

Fast path (`snapshot`): taken only when all of these hold, else fall back to `dump` and record `clone_path_reason`:

1. Mode is `full` and source is `live`.
2. The parent's data volume directory is on btrfs (the volume directory is its own subvolume) or zfs (a dataset per volume), detected at branch time from the volume mountpoint, never assumed. XFS or btrfs reflink without a subvolume (`cp --reflink=always`) is accepted only when the parent is stopped, because a per file reflink is not atomic across files.
3. The disk guard does not refuse (below).

A btrfs or zfs snapshot is atomic, so the copy is crash consistent and every listed engine recovers it with its own journal or WAL replay. The snapshot is promoted to a writable clone that becomes the branch's volume. Detection results are shown on the capability endpoint and in `levelrail doctor`.

Disk guard: add op `branch` to `internal/diskguard`. Before starting, the slow path needs free space of `parent size times branches.slow_path_headroom_factor` plus the guard's reserve; the fast path needs only the reserve. Refusal is a 507 with the `low_disk` code like every other guarded op, and the dialog shows it before submit.

### Anonymization

Rule kinds: `null`, `hash` (HMAC SHA 256 with a per ruleset salt, output truncated and re-encoded to fit the column type), `partial` (keep first N and last M characters, mask the rest with a fixed character), `fake` (values from a small built in generator: names, emails on a reserved domain, phone numbers, addresses, dates within a range, numbers within a range; seeded), `keep`. Default for a column with no rule is `keep`, and the dialog lists every column with a detected sensitive name (email, phone, name, address, token, password, ssn, iban and similar, from a code table) that has no rule, so the user decides each one.

Deterministic option: `hash` and `fake` derive from HMAC(salt, value), so the same input gives the same output across tables and joins on masked keys keep working. Non deterministic mode draws from a per run seed. The salt lives in `internal/secrets` under the ruleset, is generated at ruleset create, and is never returned by any API.

Where it runs: in the dump stream, between parent and branch, in a Go transformer in the control plane (Postgres COPY text rows keyed by the COPY column list, MySQL single row INSERTs produced with `--skip-extended-insert --complete-insert --hex-blob`, MongoDB archive BSON documents). Unmasked rows are never written to the branch volume, its WAL, its binlog or its journal. Post restore UPDATEs were rejected (below). A rule whose target is missing in the dump, or a value the transformer cannot parse, fails the branch; it never passes the value through. Anonymized mode never uses the fast path. Generated columns, triggers and foreign keys are restored after data as the dumps already order them.

Dry run: `POST .../masking-rules/preview` reads up to `branches.mask_preview_rows` sample rows per targeted table through the existing read only viewer path, applies the rules in memory and returns only the masked values plus the column list. It requires the same ability as reading rows, is audited, and the sample is never stored.

### Expiry job

A reconciler loop in the control plane, every `branches.expiry_interval`, selects `ready` or `failed` rows past `expires_at` (or past `branches.failed_retention`), skips protected rows, moves them to `expiring`, deletes them through the normal delete path, and writes an audit event. A warning notification goes out `branches.expiry_warning` before expiry. Presets offered in the dialog: 1 hour, 1 day (default for manual branches), 7 days, 30 days, never; pull request branches default to `branches.preview_default_ttl` and are also deleted with the preview. Extending expiry is a PATCH.

### Quotas

Per root: `branches.max_per_root`. Platform: `branches.max_total` and `branches.max_total_bytes` (sum of `size_bytes`). Max branch depth `branches.max_depth`. Max parent size for the slow path `branches.slow_path_max_bytes` (bigger parents need the fast path or a backup source). All checked before any container is created.

### Naming and connections

Branch name is a normal database name (same validator), default suggestion `<parent>-<short>`; pull request branches are `<parent>-pr-<number>` truncated to fit 63 characters with a hash suffix on collision. Because a branch is a normal database, its host, port, user, URL fields and `from: postgres.<name>.url` references work unchanged, and `databases connect`, tunnel and public access apply to it once `ready`. Each branch has its own generated credentials; nothing is copied from the parent's secrets.

### Reset, promote, protect, delete

- Reset to parent: refused when protected. Stops the branch, re-runs the original mode from the parent's current state (same ruleset version unless the user picks the latest), into the branch's own volume, keeps the branch's credentials so connection strings do not change. Typed confirm of the branch name. A failed reset leaves `failed` and the branch unreachable; the user retries reset or deletes.
- Promote: detaches the branch from its parent (clears `parent_name`, `expires_at`, sets `state=promoted`), so it survives the parent and the expiry job. It does not rename, does not relink apps and does not touch the parent. Relinking apps is the existing links flow, offered as a follow up step with the list of the parent's linked apps.
- Protected: blocks delete, reset, expiry and the expiry job; toggling it needs `write:sensitive`.
- Delete: typed confirm of the branch name; a branch delete discards the data volume (unlike a root delete, which keeps it), because a disposable copy that keeps its bytes defeats the disk guard. `--keep-volume` keeps the old behavior.

### Compare

`GET .../branches/{branch}/compare` returns a structured schema diff (tables, columns, types, nullability, defaults, indexes, constraints, for Postgres also sequences and views) between the branch and its parent, built from catalog queries through the read only viewer path, plus per table row count estimates and sizes. MySQL family through `information_schema`. MongoDB: collections, indexes and validators only. No DDL is applied; a generated DDL preview is read only text. Redis family and the rest: compare refused with the reason.

### Authorization

| Action | Ability |
| --- | --- |
| list, show, compare, capability | `read` on the root |
| masking preview | `read:sensitive` on the parent |
| create full, pitr, schema | `write:sensitive` on the parent (same tier as restore-as-new today) |
| create anonymized | `write:sensitive` on the parent; editing rulesets also `write:sensitive` |
| reset, extend expiry, protect toggle | `write:sensitive` on the branch |
| promote, delete | `write:sensitive` on the branch plus typed confirm; delete of a protected branch is refused for everyone |

An anonymized branch is the one case where read on the branch may be granted to a principal who lacks `read:sensitive` on the parent. That grant is explicit (the existing grants flow), never inherited.

### Audit events

`database.branch.create`, `database.branch.ready`, `database.branch.failed`, `database.branch.reset`, `database.branch.promote`, `database.branch.protect`, `database.branch.unprotect`, `database.branch.extend`, `database.branch.expire`, `database.branch.delete`, `database.masking.ruleset.save`, `database.masking.preview`. Each carries parent, branch, mode, clone path, ruleset id and version, principal. No row data and no salt ever.

### API and CLI

API under `/api/v1/databases/{name}/branches`: `POST` (create: name, mode, source, source_ref, expires preset or time, protected, ruleset id), `GET` (tree of the root), `GET /{branch}`, `POST /{branch}/reset`, `POST /{branch}/promote`, `PATCH /{branch}` (expiry, protected), `DELETE /{branch}` (confirm body), `GET /{branch}/compare`, `GET /branch-capabilities`. Masking: `GET|PUT /api/v1/databases/{name}/masking-rules`, `POST .../masking-rules/preview`. The existing POST keeps its current body working (backup_id or at maps to source `backup` or `pitr`, mode `full` or `pitr`).

CLI: `levelrail databases branch create|ls|show|reset|promote|rm|compare`, with `create <parent> <name> --mode full|pitr|schema|anonymized --from-backup ID --at TIME --expires 1d --protected --rules latest`, `rm --confirm <name>`, `--json` everywhere. The current `create` flags stay valid.

### Dashboard

A Branches page under each database (left nav: Overview, Branches, then data tools): a tree of the root with name, mode badge, size, last active, expiry countdown, protected lock, state; actions connect, reset, promote, delete with typed confirm. The Create branch dialog replaces `BranchDatabaseDialog`: name (prefilled), parent (prefilled with the current database, picker for any branch in the tree), auto delete presets, mode radio list with each unavailable mode disabled and its reason, and for anonymized a rules editor with the sample row preview. A Compare tab per branch. The database switcher shows branches nested under their root.

### MCP read tools

`list_database_branches` (tree, states, expiry, size), `get_database_branch`, `compare_database_branch` (schema diff only, no row data), `get_database_branch_capabilities`. No create, reset or delete tools in this ADR, and no tool returns masking salts or sample rows.

### Preview environments

New `spec.Database.SeedFrom` value `branch` (with `ephemeralInPreviews: true`): the preview database is created through the branch path as a child of the named database, mode from a new `previewBranchMode` field (`schema` default, `anonymized` with the database's latest ruleset, `full` only when the parent is in a non production environment), `preview_environment_id` set, expiry `branches.preview_default_ttl`, deleted with the preview teardown. `none` and `latest` keep working. `isolatedInPreviews` is unchanged and stays the cheap shared option.

### PITR and F08

The `pitr` mode is live for MySQL and MariaDB because their PITR already restores into a new database. For Postgres it stays disabled in the dialog with the reason "point in time branches need restore into a new database (F08)" until a `PITRRestorer` variant restores a base backup plus WAL into the branch's fresh volume instead of the parent's. When F08 lands, Postgres `pitr` branches reuse that runner with no model change. MongoDB and Redis family pitr stays unsupported.

### Settings

All live runtime settings, env seeds the default:

| Setting key | Env seed | Default |
| --- | --- | --- |
| branches.enabled | APP_BRANCHES_ENABLED | true |
| branches.expiry_interval | APP_BRANCHES_EXPIRY_INTERVAL | 5m |
| branches.expiry_warning | APP_BRANCHES_EXPIRY_WARNING | 2h |
| branches.default_ttl | APP_BRANCHES_DEFAULT_TTL | 24h |
| branches.preview_default_ttl | APP_BRANCHES_PREVIEW_DEFAULT_TTL | 72h |
| branches.failed_retention | APP_BRANCHES_FAILED_RETENTION | 24h |
| branches.max_per_root | APP_BRANCHES_MAX_PER_ROOT | 10 |
| branches.max_total | APP_BRANCHES_MAX_TOTAL | 50 |
| branches.max_total_bytes | APP_BRANCHES_MAX_TOTAL_BYTES | 50 GiB |
| branches.max_depth | APP_BRANCHES_MAX_DEPTH | 3 |
| branches.slow_path_max_bytes | APP_BRANCHES_SLOW_PATH_MAX_BYTES | 20 GiB |
| branches.slow_path_headroom_factor | APP_BRANCHES_SLOW_PATH_HEADROOM | 1.5 |
| branches.fast_path | APP_BRANCHES_FAST_PATH | auto (auto, off) |
| branches.activity_interval | APP_BRANCHES_ACTIVITY_INTERVAL | 5m |
| branches.size_interval | APP_BRANCHES_SIZE_INTERVAL | 15m |
| branches.mask_preview_rows | APP_BRANCHES_MASK_PREVIEW_ROWS | 5 |
| branches.clone_timeout | APP_BRANCHES_CLONE_TIMEOUT | 2h |

Last active comes from engine counters sampled every `activity_interval` (Postgres `pg_stat_database` transaction counters, MySQL `Questions`, MongoDB `serverStatus` opcounters, Redis `total_commands_processed`); a counter change since the last sample sets `last_active_at`. Size comes from the volume usage the disk history code already measures.

## Alternatives rejected

- Postgres `CREATE DATABASE ... TEMPLATE`: needs no other sessions on the source, so it blocks the live parent, and it stays inside one server, which breaks our one database per container model and resource limits.
- `pg_basebackup` for the slow path: copies the whole cluster including roles and every database, needs a replication role and slot handling on the parent. `pg_dump` already gives one consistent snapshot of the single app database we manage. Kept as an option for F08 only.
- Masking with UPDATEs after restore: unmasked values stay in dead tuples, WAL, binlog and journal files of the branch volume until vacuum or rotation, and a crash between restore and update leaves an unmasked branch on disk. Rejected even with the branch unreachable.
- Copying the parent's data volume with a plain file copy while running: not consistent; with the parent stopped it costs downtime on the parent. Only the atomic snapshot or a stopped parent qualifies.
- Promote as a name swap with the parent: names key containers, volumes, secrets, links and audit history and are immutable by design; a swap would touch all of them on a live production database.
- Building our own copy on write storage layer: out of scope; the promise is upstream engines on plain disks.
- Separate branch resources outside `desired_databases`: duplicates the reconciler, credentials, metrics, connect and backup code a branch gets for free as a normal database.

## Security

- Nothing reachable before `ready`: no network alias, no published port, no links, no reveal, no viewer, while `cloning`, `masking`, `credentials` or `verifying`. Enforced in the reconciler and in the API, not only in the UI.
- Fast path copies the parent's files, which include the engine's own user catalog and password hashes. The `credentials` step rotates every login role or user to new secrets (or drops the parent's non owner roles) before the branch can be reached. The slow path dumps carry no roles today; keep `--no-owner --no-privileges`.
- Anonymized: transform in stream, fail closed on parse errors or missing targets, salts in secrets, never in rules JSON, logs, audit or API responses. Ruleset version pinned on the branch so an audit can show exactly what was applied.
- The branch inherits the parent's project and environment scope unless the user picks another; a branch of a production database into a non production project needs `write:sensitive` on both.
- Dumps stream through the control plane helper, no temp file. Error messages go through the same scrubbing `datamigrate` uses.
- Typed confirm on delete, reset and promote; protected blocks destructive ops for everyone, including root.
- Expiry job deletes only rows it can compare and set into `expiring`; it never deletes a database without a `database_branches` row.

Security review checklist for each branch slice:

1. Branch unreachable before `ready` (network, port, link, reveal, viewer, tunnel).
2. Fast path rotates or drops every inherited login.
3. No unmasked byte written to an anonymized branch volume, including on failure.
4. Salt never leaves `internal/secrets`.
5. Authz per action matches the table, on the right resource (parent vs branch).
6. Audit row on every state change, with no data values.
7. Quotas and disk guard checked before any container or volume exists.
8. Identifier and path inputs (rule targets, names) validated against catalogs, not interpolated.
9. Delete and expiry cannot reach a root or a protected branch.
10. Preview branches never use `full` from a production parent.

## Failure modes

- Clone fails mid stream: branch goes `failed`, container stopped, not reachable; the expiry job removes it after `failed_retention`. The parent is untouched because it is only read.
- Parent goes down during a live dump: dump fails, same as above, parent state unaffected.
- Disk fills during the slow path: the guard refuses at start; if it still fills, the restore fails and the branch is `failed`; the volume is discarded on delete.
- Fast path detection is wrong or the snapshot fails: fall back to `dump` and record why; never fail the request only because the fast path failed.
- Control plane restarts mid clone: on boot, rows in `cloning`, `masking`, `credentials`, `verifying` older than `clone_timeout` move to `failed`; nothing resumes half way.
- Mongo standalone without oplog: branch is marked `best_effort` consistency and the dialog says so.
- Expiry job fails to delete: row stays `expiring` with the reason, retried next pass, surfaced on the Branches page.
- Reset fails: branch `failed`, credentials unchanged, retry or delete.

## What cannot be verified locally

- Fast path on btrfs and zfs: the development machines run Docker Desktop's VM on ext4, so detection and snapshot code needs a real Linux host with those filesystems (one of the test hosts, provisioned for it).
- MongoDB `--oplog` consistency needs a replica set parent; the managed MongoDB is a standalone today, so the default case is `best_effort`.
- Large parent timings and headroom factors: need a real host with tens of GiB.
- Postgres pitr branches: blocked on F08.
- Whether preview teardown discards ephemeral database volumes today was not confirmed in code; the preview slice checks and fixes it.

## Testing plan

- Unit: state machine transitions and compare and set races; capability matrix per engine; quota and disk guard math; naming truncation; each masking kind, deterministic stability, type fitting, and fail closed parsing (COPY escapes, MySQL hex and quoted strings, nested BSON); salt never serialized.
- Store: migrations apply, parent with children cannot be deleted, indexes used by the expiry query.
- API: authz table per route, typed confirm, protected refusals, 507 path, legacy POST body still works.
- Live tests (existing `_live_test.go` pattern, gated): Postgres, MySQL, MariaDB, MongoDB, Redis slow path full and schema; anonymized Postgres and MySQL with a scan of the branch volume files for known plaintext seeds; reset keeps credentials; delete frees the volume.
- Real host: fast path on btrfs and zfs with a write load on the parent during the snapshot, then engine recovery and row count check.
- One tutorial run through the dashboard as QA: create, compare, reset, preview branch per pull request, expiry.

## Build slices

| # | Slice | Files (owner) | Depends | Review |
| --- | --- | --- | --- | --- |
| 1 | Store: migrations and store methods for branches and masking rulesets | `internal/store/migrations/NNNN_database_branches.sql`, `NNNN_database_masking_rules.sql`, `internal/store/database_branches.go`, `database_masking.go` | none | no |
| 2 | Lifecycle core: state machine, capability matrix, quotas, settings keys, diskguard op `branch` | `internal/dbbranch/` (new), `internal/diskguard/diskguard.go`, `internal/runtimesettings` keys | 1 | lr-reviewer |
| 3 | Live slow path clone with isolation until ready and the credentials step, all five families | `internal/backup/branch_live.go`, `dump.go` (consistent flags), `internal/reconcile/database/controller.go` (isolation flag) | 2 | lr-reviewer |
| 4 | API: create, list, show, patch, delete, reset, promote, capabilities; legacy body mapping | `internal/api/database_branch*.go`, `routes_platform_backup.go` | 3 | lr-reviewer |
| 5 | CLI verbs create, ls, show, reset, promote, rm, compare | `cmd/levelrail-cli/databases_branch*.go`, `internal/apiclient` | 4 | no |
| 6 | Expiry job, activity and size sampler, warning notifications | `internal/dbbranch/expiry.go`, `activity.go`, `cmd/levelrail/main.go` wiring | 4 | no |
| 7 | Schema only mode per engine | `internal/backup/branch_live.go` flags, capability matrix | 3 | no |
| 8 | Anonymization: rules API, stream transformers, generator, preview | `internal/dbmask/` (new), `internal/api/database_masking.go` | 3, 1 | lr-reviewer |
| 9 | Dashboard: Branches page, Create branch dialog, rules editor | `web/src/routes/databases/$name/branches.tsx`, `web/src/components/databases/branches/`, locales | 4, 8 | no |
| 10 | Compare: schema diff and counts, API, CLI, Compare tab | `internal/dbbranch/compare.go`, `internal/api/database_branch_compare.go`, web compare tab | 4 | no |
| 11 | Preview environments: `seedFrom: branch`, `previewBranchMode`, teardown discards volume | `internal/spec/spec.go`, `validate.go`, `internal/api/preview_environments_databases.go`, teardown | 6, 8 | lr-reviewer |
| 12 | Fast path detection and snapshot clone, doctor check; MCP read tools | `internal/dbbranch/fastpath.go`, `internal/backup/branch_snapshot.go`, `internal/mcptools/tools_database_branches.go` | 3 | lr-reviewer |

Order: 1, 2, 3, 4, then 5, 6, 7 and 10 in parallel (no shared files), then 8, 9, 11, 12. Slices 9 and 11 both touch locale files and must not run at the same time.

## Consequences

- Every supported engine gets the same dialog, with honest per engine availability instead of a Postgres only feature.
- A branch costs a full copy of disk on hosts without btrfs or zfs; quotas and the guard keep that bounded, and the UI never says "instant" for the dump path.
- Branch delete frees disk, which differs from root delete; the dialog and docs say so.
- The anonymizer is new parsing code on the data path and the largest review surface in this ADR.
- The legacy branch POST and CLI flags keep working, so existing scripts do not break.

## Open questions

1. Should the managed MongoDB run as a single node replica set by default so `--oplog` branches are consistent, at the cost of a restart for existing databases?
2. Is `full` from a production parent allowed for manual (non preview) branches by any `write:sensitive` holder, or only root?
3. Should branch size count against the parent's project resource quota, or only the platform branch quota?
4. Is the `hash` truncation acceptable for unique constraints, or do we need a collision check pass that fails the branch?
5. Promote: is detach enough, or does the owner want an explicit "replace parent" flow with downtime in a later ADR?
6. F08 timing: is Postgres restore into a new database in scope for 1.5.11 or the next release?
7. Should ClickHouse branching be attempted once restore into a fresh server is proven, or stay refused?
