# ADR 007: Embedded SQLite in WAL mode for control plane state

Status: Accepted

Date: 2026-08-11

## Context

The control plane needs somewhere to persist desired-state resource
records (the input to the reconciler, ADR 002), deploy history, secrets
metadata, and eventually certificate state (ADR 005) and node/mesh
membership (ADR 006). The project commits to a single static Go binary
with the frontend embedded via `embed.FS` and no Node runtime on the
server (ADR 001); this decision extends that commitment to the state layer specifically.

`internal/store` (schema, migrations, queries) has since been built.
The project's engineering process treats the database schema, like
anything touching secrets, as work that needs one coherent design session
and careful human review rather than being split into independently-scoped
pieces. Unlike the BuildKit and Caddy Phase 0 spikes
(ADRs 004, 005), which were built as self-contained, non-sensitive,
independently-scoped pieces of work, this one was built and reviewed as a
single continuous unit, in the same session as this ADR's writing. See the Verified section below.

## Decision

Control plane state lives in an embedded SQLite database running in WAL
(write-ahead log) mode, using `modernc.org/sqlite` specifically rather than
`mattn/go-sqlite3` or another cgo-based driver. `modernc.org/sqlite` is a
pure-Go, no-cgo implementation, which matters here for a concrete reason:
Levelrail ships as a single static binary that has to cross-compile
cleanly for whatever Linux architectures it targets (4.1's whole premise).
A cgo dependency means the build needs a working C toolchain for every
target platform, which is exactly the kind of build-matrix complexity a
pure-Go driver avoids entirely.

Schema migrations run through an embedded migration tool, versioned and
forward-only, no down-migrations relied on in production. A Postgres driver
gets added in a later phase, and only if a highly-available control plane
becomes a real, asked-for requirement, not preemptively. Control plane
write volume is small enough (resource records, deploy history, not
application data) that SQLite's single-writer model is not a bottleneck for
the target scale (3 to 50 services, the project's stated target scale).

## Rejected alternatives

- **Postgres or MySQL as the default state store**: rejected not because
  a relational server can't do the job, but because of what standing one
  up costs operationally, and that cost is exactly what 4.1's single-binary
  story exists to avoid. `prior-art-coolify.md` is the concrete
  illustration: Coolify's stated stack is "Laravel 12 (Laravel 10 file
  structure) + Livewire 3 + Alpine.js, MySQL/Postgres, Redis, Horizon queue
  workers, Soketi websockets, Traefik proxy, optional 'Sentinel' metrics
  agent." State and queueing alone pull in three separate always-on
  services: a relational database server, Redis (backing both cache and
  Horizon's queue), and Soketi for websocket broadcast. None of that is
  Coolify's application logic, it's infrastructure the state and
  notification layer requires just to function, running as separate
  processes with their own failure modes, their own upgrade cycles, and
  their own resource floor, before a single user app is deployed. That's
  precisely the multi-process complexity 4.1 is designed to keep out of
  Levelrail, and state storage is where it would sneak back in if the
  default were relational-server-based: everything else in this project
  (4.5's embedded Caddy, 4.1's embedded frontend) is a fight against the
  same failure mode, a dependency graph that turns "one binary" into "one
  binary plus a docker-compose file."
- **Deferring Postgres support entirely, never adding it**: not what 4.7
  or this ADR decide. The rejection is of Postgres or MySQL as the
  *default*, not a permanent rejection of the option. 4.7 keeps a Postgres
  driver as a later-phase addition, gated on HA control plane becoming a
  real request rather than a hypothetical one, so the single-binary
  default stays honest for the common case without foreclosing the
  uncommon one.

## Consequences

- The control plane binary has zero external state dependencies to stand
  up before first run: no database server to provision, no connection
  string to configure, no network hop between the control plane and its
  own state. This is a meaningful chunk of what makes 4.1's single-binary
  claim true rather than aspirational.
- WAL mode is required, not optional, because it's what makes concurrent
  reads (API requests, reconciler reads) not block on writes (reconciler
  status updates, deploy history inserts) in a single-file database. Any
  code path that opens the SQLite connection outside `internal/store`'s
  configured connection needs to inherit the same WAL/pragma settings, or
  this guarantee silently breaks per-connection.
- Migrations being forward-only means a bad migration can't be walked back
  automatically; recovery from a bad schema change is a new forward
  migration, not a down-migration, which has to be a documented operator
  procedure before Phase 1 ships anything real (thesvg.org, per the Phase 1
  exit criterion).
- Backups (S3-compatible, Phase 4) need to account for SQLite's file-based
  nature directly: a consistent backup of a WAL-mode database needs either
  the SQLite backup API or a checkpoint-then-copy step, not a naive file
  copy of the main database file while it's open.
- If a Postgres driver is added later, `internal/store`'s query layer has
  to be written against an abstraction that doesn't assume SQLite-specific
  SQL dialect from the start, or that addition becomes a rewrite instead of
  an addition. Worth deciding the query-layer shape (raw SQL with a thin
  wrapper vs. a query builder) with that in mind even though Postgres
  support itself is out of scope now.

## Verified

`internal/store` implements `Open` (WAL mode, foreign keys on, a single
shared connection to avoid this process's own goroutines contending with
each other), a hand-rolled forward-only migration runner reading embedded
`migrations/*.sql` files (no external migration library added, matching
the project's "no heavy framework" engineering standard), and the first real
migration and repository: `reconcile_status`, persisting the status
conditions ADR 002's reconciler already produces. `reconcile.Engine` got a
small, additive `Store` interface (defined in `internal/reconcile`, not
imported from `internal/store`, so there's no import cycle) and an
optional `SetStore`; a store failure is logged but never fails the
reconcile itself, since converging the actual resource has to stay the
primary job regardless of whether persisting its status succeeded.

Verified against a real database, not just unit-tested against fakes:
`TestOpen_WALModeEnabled` and `TestOpen_ForeignKeysEnabled` query
`PRAGMA journal_mode` and `PRAGMA foreign_keys` directly against a real
temp SQLite file. `TestOpen_RunsMigrationsAndIsIdempotent` opens the same
file twice, confirming a restart doesn't re-run or fail on an
already-applied migration.

Also run live, not just in the test suite: `cmd/levelrail` now opens the
store at startup (path from `APP_DATA_DIR`, matching the project's
convention of overriding filesystem paths via env vars) and calls `engine.SetStore(db)`. Running the Phase 0
demo end to end and querying the database directly afterward:

```
sqlite3 data/levelrail.db "SELECT * FROM reconcile_status;"
nginx-demo|Ready|True|AlreadyRunning||2026-08-12T01:22:20.306Z
```

matching the engine's own log line for that reconcile exactly. Killing the
demo container by hand again updated the row to `Restarted` and then back
to `AlreadyRunning` on the follow-up event-triggered reconcile, live,
confirming status persistence survives the same kill-and-recover cycle
ADR 002 already verified for the reconciler itself.

Not yet done, left for Phase 1: `applications`, `deployments`, `servers`,
and the rest of the domain schema the reconciler (ADR 002) and the app
spec (`app.yaml`) describe.
Inventing those tables now, ahead of the code that would actually use
them, would be exactly the kind of speculative schema this project's own
standard against designing for hypothetical future requirements
rules out; `reconcile_status` was chosen as the first real migration
specifically because it persists something that already exists in the
codebase, not something anticipated.
