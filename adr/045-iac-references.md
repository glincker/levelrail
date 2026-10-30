# ADR 045: Database references and jobs in platform documents

Status: Accepted (experimental in v0.8.6, addendum to ADR 042)

Date: 2026-10-25

## Context

An App document could not say "this env var is the connection string of that
Database". Migrations emitted every connection variable as a `secretRef`
placeholder plus a `SECRETS.md` note, and the operator copied the value by
hand. Scheduled jobs were rejected by apply and became migration notes.

The control plane already supports both: `store.DesiredService.DatabaseEnv`
holds `{database, field}` references that the application controller resolves
right before container creation, set through `POST /api/v1/apps/{name}/connections`,
and jobs have their own API under `/api/v1/apps/{name}/jobs`.

## Decision

1. **Syntax.** An App env value may be `{ from: { database: NAME, field: FIELD } }`.
   `field` is `url` (default), `host`, `port`, `username`, `password` or
   `database`. The app.yaml string form `{ from: NAME.FIELD }` is accepted so a
   service can be copied from app.yaml unchanged. It sits next to `secretRef` as
   one more object branch of the env value schema. The object form names the
   target kind (`database`), so other kinds can be added later without a
   new keyword.
2. **Resolution happens at container create, not at plan or apply.** Apply
   writes the reference through the connections API. The plan, its hash, the
   stored revision documents and exports hold only `NAME.FIELD`. Nothing in
   `internal/iac` ever reads a database credential, so the stack secret guard
   tests need no new case.
3. **Ordering.** The plan fails an app whose referenced database is neither live
   nor declared, lists the referenced databases in a new `dependsOn` field on the
   change, and Database items keep running before App items (kind order).
4. **Jobs.** `service.jobs` is applied by name through the jobs API with the same
   defaults the deploy pipeline uses for app.yaml jobs. Undeclared jobs are kept,
   or removed by prune on a managed app. Jobs whose source is app.yaml are left
   alone with a warning. An App without `jobs` never touches jobs.
5. **Emitters** turn a connection variable into a `from` reference only when
   exactly one emitted database of the matching engine exists, and turn source
   cron entries into jobs unless the command looks like it carries a credential.

## Deferred

- **Volumes** (done, wave Q): `service.volumes` is applied by name through the existing
  `PUT /api/v1/apps/{name}/volumes`. Named volumes only, kept when unlisted, detached
  (never deleted) by prune. Still deferred: host-path volumes, size hints and deleting
  a volume's data from apply.
- **`vault` env references, egress policies, GPU, registry credentials**: no
  change, still rejected.
- **References to other kinds** (an app's internal URL, a domain): no runtime
  resolver exists yet.
- **Exporting jobs**: export writes `from` references but not jobs yet.
- **Readiness and port** needed no change: `port` and `health` were already applied.

## Consequences

Secret exposure stays at names only. One accepted risk: a job command is plain
text in the files and in the plan. Commands of live jobs that look like a
credential are hidden as `(hidden)` in the plan, and the migration emitters keep
such commands out of the files.
