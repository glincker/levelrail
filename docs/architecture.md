# Architecture

This is how Levelrail is actually built today, not just how the phase plan
describes it. Where "shipped" and "designed for later" differ, this page
says which one you're looking at.

## Control plane

Package: `internal/reconcile` (see `internal/reconcile/engine.go`, and the
per-resource controllers under `internal/reconcile/application`,
`internal/reconcile/database`, `internal/reconcile/ingress`,
`internal/reconcile/mesh`, `internal/reconcile/nodehealth`).

The core loop is a reconciler, not an imperative deploy script. Desired
state lives in the database as declarative resource records; each
controller diffs desired against observed Docker state and converges the
two. The pattern is borrowed from Kubernetes controllers without pulling
in the rest of the Kubernetes runtime.

Every controller is idempotent and level-triggered, never edge-triggered.
`Reconcile` never assumes it's continuing a previous partial operation: it
re-derives what to do entirely from current observed state on every call.
That's what makes a reconciler safe to interrupt and safe to retry: calling
it again after a half-finished attempt converges correctly instead of
double-applying or corrupting state. Every reconcile also emits a status
condition with a reason string, so a failed convergence shows up in the UI
with an explanation, not just a spinner that never resolves.

## Node agent

Packages: `internal/agent` (see `internal/agent/transport.go`,
`internal/agent/grpc_transport.go`, `internal/agent/server.go`) and
`internal/docker` (see `internal/docker/client.go`).

The node agent dials **out** to the control plane rather than the control
plane dialing in, so managed servers never need an inbound port open. It
talks to the local Docker Engine API directly; there's no shelling out to
the `docker` CLI anywhere in this path.

The transport is an interface (`agent.Transport`), and two implementations
satisfy it: `Local`, which calls a `docker.Runtime` in-process, and
`GRPCTransport`, which dispatches the same calls over a real reverse-dialed
gRPC connection with mTLS. Both are shipped and in use, not just the
in-process one: node enrollment (one-time join tokens exchanged for a
client certificate), a real `levelrail-agent` binary, and cross-node
service placement all work today. Reconcilers and everything above the
transport boundary don't know or care which implementation they're talking
to: that's what keeps single-node and multi-node the same code path.

Also shipped in this layer: a WireGuard mesh (`internal/network`, built on
`wireguard-go`) that gives every node a peer and internal DNS names that
resolve across machines, and dedicated build-node support (see Builds,
below) so builds don't compete with production containers for CPU on a
given box.

## Builds

Package: `internal/build` (see `internal/build/client.go`,
`internal/build/solve.go`).

Builds go through BuildKit's Go client, not `docker build`. That's what
makes remote build cache and parallel stage execution available at all;
shelling out to the `docker build` CLI would give up both. Dockerfile
builds are supported directly via the `dockerfile.v0` frontend, and
Railpack (`github.com/railwayapp/railpack`) auto-detects a build plan for
apps that don't ship a Dockerfile.

Remote cache is registry-backed (`internal/build/cache.go`), which is what
lets a fleet of dedicated build nodes share cache state instead of each
one rebuilding from scratch. `build.type: static` is also supported as its
own path with no container involved, matching the app-spec's `static`
build type.

## Ingress

Package: `internal/ingress` (see `internal/ingress/driver.go`,
`internal/ingress/certstorage.go`).

Caddy (`github.com/caddyserver/caddy/v2`) is embedded as a library and
driven in-process through its admin API, rather than run as a separate
container with its own config surface. That buys automatic TLS and
HTTP/3 without a second moving part to operate. Certificate storage lives
in the database (`internal/ingress/certstorage.go` implements
`certmagic.Storage` over `internal/store`) so certs are shared across
nodes rather than pinned to whichever machine issued them.

TLS today defaults to an internal, self-signed issuer. A real Caddy ACME
issuer, a settings toggle, and form validation are built and wired end to
end; the one open gap is that issuance has only been unit-tested at the
config level, not yet spot-checked against a real domain issuing a real
cert.

## State

Package: `internal/store` (see `internal/store/store.go`,
`internal/store/database.go`, `internal/store/migrate.go`).

State is embedded SQLite in WAL mode, opened via `modernc.org/sqlite`
(pure Go, no cgo). That's what keeps cross-compiling the control plane
binary simple. Schema changes go through embedded, versioned, forward-only
migrations.

## Observability

Package: `internal/telemetry` (see `internal/telemetry/collector.go`,
`internal/telemetry/logs.go`, `internal/telemetry/federate.go`).

Each node keeps its own local time-series store (15s resolution, CPU,
memory, disk IO, network IO, deploy count, build duration) and its own
local log store with full-text search (FTS5) and structured JSON log
parsing. Nothing is centrally indexed by default, which is what keeps idle
cost near zero as the number of managed apps grows.

Federated query is real, not aspirational: `internal/telemetry/federate.go`
defines a `MetricsSource` interface that both the local, single-node store
and a remote node's data satisfy identically, and the query API
(`internal/api`) fans a query out and merges results across nodes. A
Prometheus remote-read endpoint exposes the same data for anyone who wants
to point their own Grafana at it.

Also shipped on top of the metrics/log layer: threshold-based alerting
with webhook, Slack, Discord, email, and Telegram notification channels,
and crashloop detection that surfaces the last 200 lines of a failing
container's logs directly in the UI.

## Secrets

Package: `internal/secrets` (see `internal/secrets/manager.go`,
`internal/secrets/dek.go`).

Secrets use envelope encryption: a per-app data encryption key, wrapped by
a master key the control plane holds. Agents receive decrypted
environment variables only at container-create time and never persist
them to disk. The crypto primitives come from `filippo.io/age`.

## Frontend

Directory: `web/` (React, Vite, TypeScript, Tailwind, TanStack Router and
Query).

The frontend builds to static assets and is embedded into the control
plane binary via `embed.FS`, so there's no separate Node process to run in
production alongside it. Route-level code splitting
keeps, for example, the log viewer's bundle out of the initial dashboard
load. Live build logs and app log tailing use server-sent events rather
than websockets, because SSE reconnects cleanly through proxies without
extra client-side plumbing.

## What's still ahead of the code

A few pieces described in the platform's design aren't finished yet,
worth naming plainly rather than leaving implicit:

- Public ACME certificate issuance, verified against a live domain.
  Ingress defaults to an internal, self-signed issuer; a real Caddy
  ACME issuer, settings toggle, and form validation are built and
  wired end to end, but only unit-tested at the config level so far.
