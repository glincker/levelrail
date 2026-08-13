# ADR 009: SQLite-backed metrics store with an in-memory recent-window cache

Status: Accepted

Date: 2026-08-12

## Context

ADR 008 decides the architecture (node-local storage, federated pull
query) but explicitly leaves the storage engine open: CLAUDE.md 4.8
says "Evaluate embedded VictoriaMetrics vs a purpose-built ring buffer,"
a real evaluation, not a rhetorical one. This ADR makes that call for
TASKS.md 2.1.

Two options on the table, per CLAUDE.md 4.8's own framing:

1. **Embed VictoriaMetrics's storage engine as a Go library**
   (`VictoriaMetrics/lib/storage`). Real upside: it's a proper
   time-series database, with compression and a wire-compatible
   Prometheus remote read/write implementation already built in, which
   would make TASKS.md 2.6 (Prometheus remote read endpoint) nearly
   free instead of a translation layer to write from scratch.
2. **A purpose-built store**, custom schema and query logic sized to
   this project's actual access pattern (one service's container
   metrics, 15s resolution, 15-day default retention, single-node
   query volume today, federated fan-out later per ADR 008), not a
   general-purpose multi-tenant TSDB's feature set.

## Decision

Purpose-built, and specifically: SQLite-backed (a dedicated
`telemetry.db`, separate from `internal/store`'s `levelrail.db`, see
Consequences) for durable retention and range queries, with an
in-memory ring buffer per (resource, metric) series in front of it as a
write-behind cache for the live dashboard's most recent window (roughly
the last 5 minutes), so a live chart never waits on a disk read for the
data it's most likely to be asking for right now.

Every other architectural decision in this codebase already picks the
minimal, dependency-light option over the more capable but heavier one
doing the same job: BuildKit as a library instead of shelling `docker
build` (ADR 004), Caddy embedded instead of a separate Traefik
container (ADR 005), `modernc.org/sqlite` specifically for being
pure-Go with no cgo toolchain requirement (ADR 007), Railpack over
Nixpacks "for being significantly lighter" (CLAUDE.md 4.4). CLAUDE.md
4.1 states the reason once, directly: "do not pull in a heavy
framework." VictoriaMetrics's storage engine is the heaviest option on
the table here by a wide margin, and reusing SQLite (a dependency this
binary already ships and already knows how to migrate, per ADR 007)
costs nothing new instead of adding one.

## Rejected alternatives

- **Embedded VictoriaMetrics.** Two independent problems, either one
  alone would be enough:
  - `lib/storage` is not a package VictoriaMetrics's own maintainers
    support as a standalone embeddable library. It is an internal
    implementation detail of their server binary, with no public API
    stability guarantee across versions. Importing it directly means
    tracking an upstream project's internal churn as if it were a
    public dependency, a maintenance liability with no equivalent
    anywhere else in this codebase's dependency choices.
  - Its feature set (multi-tenant isolation, downsampling rollups,
    deduplication across replicated writers) solves problems this
    control plane doesn't have at the scale CLAUDE.md 6 actually
    targets (3-50 services, 1-10 machines). Bringing in that surface
    area to get compression and a free Prometheus endpoint is paying
    for capability the product doesn't need to buy the one or two
    pieces it does.
- **A pure in-memory ring buffer with no persistence at all**, the
  most literal reading of CLAUDE.md 4.8's phrase. Rejected on a
  concrete number: 15 days at 15s resolution is 86,400 samples per
  series; at roughly 16 bytes/sample (timestamp + value), a modest
  deployment of 50 services x 10 metrics each is already ~500 series,
  ~700MB of metric history alone, held in RAM for the entire retention
  window, on a control plane CLAUDE.md 6 Phase 5 explicitly wants to
  measure idle memory for ("100 apps on one control plane, measure
  idle CPU and memory, publish the number"). That number would be
  dominated by an in-memory retention buffer that didn't need to be
  one. It also loses all metric history on every restart, which
  Consequences below accepts as a real trade-off for the SQLite-backed
  choice too, but an unbounded in-memory-only design pays the worst of
  both: no durability *and* the memory cost of long retention.
- **A custom on-disk format** (a hand-rolled append-only chunked file
  per series, closer to what "ring buffer" usually implies for a
  disk-backed TSDB). Rejected specifically for this project: it means
  writing and testing a bespoke storage format's write path, crash
  recovery, and compaction from scratch, exactly the kind of
  infrastructure SQLite in WAL mode already provides for free and ADR
  007 already committed this binary to depending on. Revisit only if
  SQLite's write throughput at real scale (Phase 5's chaos/performance
  testing) turns out to be the actual bottleneck, not before.

## Consequences

- A **separate SQLite file** (`telemetry.db`), not a new table in
  `internal/store`'s existing `levelrail.db`. Metric writes are
  frequent and high-volume relative to `levelrail.db`'s desired-state
  writes (ADR 007's whole write volume, "tiny," describes config
  changes, not a 15-second collection tick across every running
  container); mixing that write pattern into the same WAL file risks
  checkpoint pressure and lock contention against the control plane's
  actual state-of-record. Two files, two independent write paths, one
  process.
- The in-memory ring buffer is a cache, not a second source of truth:
  a process restart loses only the live-dashboard warm cache, not
  retained history, since SQLite already has it. This is a real
  advantage over the rejected pure-in-memory option, not a
  compromise.
- Retention enforcement is a periodic delete sweep
  (`DELETE FROM ... WHERE ts < ?`), not a rolling-file-deletion or
  downsampling scheme; simple, and sized correctly for this project's
  actual data volume (Rejected alternatives above), not for
  VictoriaMetrics-scale ingestion rates.
- TASKS.md 2.6 (Prometheus remote read) is real work, not close to
  free: a translation layer from this SQLite schema's query results
  into the Prometheus remote read wire format, same conclusion ADR 008
  already reached independently ("has to be built as a real
  translation layer over the federated query path, not a shortcut").
  This ADR doesn't change that; embedding VictoriaMetrics was the only
  path that would have, and it was rejected above on other grounds
  first.
- If a future scale target genuinely outgrows SQLite for this (many
  more nodes, much higher metric cardinality than Phase 1-3's stated
  target), the schema this ADR commits to should make that visible
  through slow queries or write contention under Phase 5's performance
  testing, not through a guess made now; that's an explicit signal to
  revisit this ADR later, not a gap in it today.
