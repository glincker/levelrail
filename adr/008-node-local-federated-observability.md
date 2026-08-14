# ADR 008: Node-local metrics and log storage, federated pull-query

Status: Accepted

Date: 2026-08-11

## Context

The project names this the differentiator, not a feature among
others: "Neither treats observability as a first-class primitive. Metrics
and logs are bolted on, usually by telling the user to install Grafana as
a one-click app." This ADR is the concrete architecture behind that
claim, and it's a specific rejection of the obvious default (ship
everything to a central time-series store) in favor of keeping metrics and
logs where they're generated and querying them in place.

This is the strongest ADR to write up because Phase 0 research directly
priced out what the rejected alternative costs a real competitor, not in
the abstract but in code and in its own issue tracker.

## Decision

Each node agent keeps its own local time-series store for container
metrics (CPU, memory, disk IO, network IO, request rate, response time
percentiles, error rate, restart count) and its own local log store with
full-text search, built by reading Docker's log stream directly, chunking,
compressing, and indexing it rather than reading the json-file driver's
files off disk. Retention is configurable per node, defaulting to 15 days
at 15-second resolution.

The control plane never centrally ingests or indexes this data. Instead it
fans a query out to the relevant agents and merges results at query time;
query is pull, not push. Idle cost stays near zero specifically because
nothing is being shipped to or centrally indexed by the control plane
between queries. A Prometheus-compatible remote read endpoint is exposed
on top of this so an operator who wants Grafana anyway can point it at
Levelrail without Levelrail needing to run or manage Grafana itself.

## Rejected alternatives

- **Centralized push, ship every node's telemetry to the control plane
  continuously**: rejected because it reintroduces exactly the always-on,
  scales-with-node-count cost this design exists to avoid; a federated
  pull query only does work when someone asks a question, a centralized
  push pipeline does work constantly whether anyone is looking or not.

- **SSH/CLI-polling as the mechanism for observing state, Coolify's actual
  v4 architecture**: `prior-art-coolify.md` documents the scheduler in
  concrete detail (`app/Console/Kernel.php:38-99`): `ServerManagerJob` runs
  every minute and iterates all registered servers, dispatching a
  connection check plus `ServerCheckJob`, which runs
  `docker container inspect $(docker container ls -aq) --format '{{json .}}'`
  over SSH against every server (`app/Models/Server.php:1006`), plus a
  proxy health check. The research doc's own summary is direct: "with zero
  apps deployed but at least one server added, Coolify SSHes into that
  server roughly once a minute... forever... There is no Docker
  event-stream subscription anywhere in this codebase, every state read is
  a fresh SSH exec." That's not observability at all, it's the general
  polling architecture Coolify uses for state, but it's the same failure
  shape 4.8 is designed against: cost that scales with server count and
  runs continuously regardless of whether anyone is watching, versus
  Levelrail's node-local-store-plus-pull-query, where idle cost stays flat
  because nothing runs unless queried.
  - `prior-art-coolify.md` also documents the highest-signal real-world
    cost example in this category: issue
    [#2110](https://github.com/coollabsio/coolify/issues/2110),
    "Unexplained High CPU Usage Spike Following Deployment Attempt" (166
    comments, open, labeled Confirmed Bug), CPU spiking to 300%+ after a
    failed deploy and persisting across reboots. It's worth being precise
    here the way the research doc itself is precise: the issue thread
    never conclusively pins the root cause on the SSH-polling scheduler
    specifically, and this ADR is not claiming it does. What #2110
    establishes on its own is weaker but still real: a 166-comment,
    long-open, confirmed CPU-spike bug exists in a codebase whose
    documented architecture is continuous per-server SSH polling with zero
    event-stream usage, which is exactly the category of idle/background
    CPU burn Levelrail is positioned against. Cite the
    code (the every-minute `ServerManagerJob` plus per-server
    `docker container inspect` over SSH), not the issue, as the actual
    evidence; the issue is context, not proof.

- **Coolify's "Sentinel" optional metrics agent, i.e. the bolted-on
  pattern generically**: this is the concrete instance of the exact
  pattern the project criticizes in the abstract ("telling the
  user to install Grafana as a one-click app"). `prior-art-coolify.md`
  describes Sentinel as "Coolify's own lightweight Go binary, not literally
  Grafana, but the same architectural pattern: opt-in, separately
  installed, and the UI explicitly gates which metrics can be shown by
  what Sentinel happens to expose." The research doc quotes Coolify's own
  `DESIGN.md:649-652` directly: "Only add a metric if Sentinel exposes
  historical data for it." That's a real engineering constraint stated
  plainly by the maintainers themselves, and it shows exactly what a
  bolted-on, opt-in observability layer costs in practice: the product's
  own dashboard capability is gated by whether a separate, optional agent
  happens to expose the data, rather than the platform owning metrics
  collection as a day-one primitive the way 4.8 requires ("Metrics that
  must exist per app without configuration"). Levelrail's node-local store
  is not optional and is not a separate agent to install; it's part of the
  same node agent that already has to be running for the reconciler and
  the Docker event stream (ADR 002, 4.3) to work at all.

## Consequences

- The control plane's query layer has to handle partial results
  gracefully: fanning a query out to N agents and merging results means
  designing from the start for an agent being unreachable, slow, or
  temporarily behind on indexing, and deciding what "the dashboard" shows
  when that happens, rather than treating every agent as always available.
- Retention and resolution are per-node config, not a single global
  setting the control plane enforces, which means a node with tighter disk
  constraints can run a shorter retention window without that being a
  control-plane-wide decision. This also means there is no single "how
  much history do we have" answer without asking every relevant node.
- The Prometheus remote read endpoint has to be built as a real
  translation layer over the federated query path, not a shortcut that
  quietly reintroduces centralized storage; it's an integration point for
  operators who want Grafana anyway, not a second, competing storage
  architecture.
- The deploy-marker-overlaid-on-metrics feature called out in Phase 2
  ("that overlay is the feature that makes the product feel different")
  depends on this ADR's decision working correctly under federation: a
  deploy event and the metric series it should overlay onto may live on
  different agents in a multi-node deployment, so the merge logic has to
  correlate across nodes, not just within one.
- Because nothing is centrally indexed, there is no single database to
  restore from if a node is lost; that node's metric and log history is
  gone with it unless a separate backup story is designed for node-local
  telemetry, which is not addressed by 4.8 or this ADR and should be an
  explicit open question before Phase 2 ships.
