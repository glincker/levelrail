# ADR 006: WireGuard mesh replaces Swarm overlay networking

Status: Accepted

Date: 2026-08-11

## Context

Once Levelrail manages more than one node (Phase 3), containers on
different machines need to reach each other by a stable name, the way a
Swarm overlay network or a Kubernetes CNI would provide. The custom
reconciler decision (ADR 002) already rejects Swarm as the orchestration
substrate; that decision would be incomplete without also deciding what
replaces the networking Swarm would otherwise have supplied for free,
since the project explicitly rules out building a full service mesh
("WireGuard plus DNS is enough").

This ADR is being written in Phase 0, well ahead of the implementation it
describes. The plan is explicit that multi-node networking is not
Phase 1 work: "Do not build this in Phase 1. Do design the network
abstraction in Phase 1 so it can be swapped in." Nothing under
`internal/network` exists yet, and Phase 3 (the phase this ADR's decision
actually gets implemented in) has not started. So unlike ADR 002, this ADR
records a design intent validated by competitor research, not a working or
tested system. There is no Verified section here because there is nothing
running yet to verify; that's expected at this stage, not an omission.

## Decision

Each managed node gets a WireGuard peer. The control plane distributes peer
configuration (public keys, allowed IPs) to every node so they form a full
mesh, not a hub-and-spoke topology through the control plane itself. Apps
get stable internal DNS names that resolve to the right peer regardless of
which node they're currently running on, so a database connection string
or an inter-service call keeps working across a node move.

Implementation uses `wireguard-go`, the userspace Go implementation, rather
than requiring the in-kernel WireGuard module, with kernel module detection
used to take the faster in-kernel path when it's available. This keeps
Levelrail's node agent portable to hosts where loading a kernel module
isn't an option (restricted kernels, some container-based VPS hosts, older
distributions) without giving up the performance of the kernel path where
it exists.

Per 4.6, this is explicitly deferred: not built in Phase 1, but the network
abstraction inside `internal/network` and wherever the agent-side transport
interface (4.3) touches it gets designed in Phase 1 so the real WireGuard
mesh can be swapped in during Phase 3 without a rewrite of everything that
depends on it.

## Rejected alternatives

- **Docker Swarm overlay networking**: rejected for the same root reason
  Swarm itself is rejected as the orchestration substrate (ADR 002):
  it's in maintenance mode, and adopting it here would
  reintroduce exactly the coupling ADR 002 already ruled out, just at the
  networking layer instead of the orchestration layer. `prior-art-dokploy.md`
  documents concretely what that coupling costs a real competitor built on
  Swarm:
  - Dokploy creates `dokploy-network` as `docker network create --driver
    overlay --attachable` at install time (`server-setup.ts:460`), and
    compose "stack" deployments create their own overlay networks
    (`utils/builders/compose.ts:60`). Plain Docker Engine API, which is
    all Levelrail's reconciler talks to (ADR 002), has no overlay driver
    activation and no routing mesh at all; a fleet of independent Docker
    daemons has no shared network concept unless something external
    provides one.
  - Dokploy's published ports rely on `EndpointSpec.Ports` with
    `PublishMode: "ingress" | "host"` (`utils/docker/utils.ts:641-652`),
    which depends on Swarm's routing mesh to forward a request arriving at
    any node to whichever node currently runs the task. `prior-art-dokploy.md`'s
    issue-tracker findings tie this directly to a real, high-signal
    failure: [#592](https://github.com/Dokploy/dokploy/issues/592),
    "Gateway Timeout on Docker Swarm worker replicas" (36 comments,
    closed), where the routing mesh doesn't correctly reach a task running
    on a non-manager node.
  - Swarm also bundles cluster bootstrap and node join
    (`docker swarm init` / `docker swarm join`, `server-setup.ts:449`,
    `cluster.ts:110/143`) into the same subsystem as its networking and
    scheduler. `prior-art-dokploy.md` is explicit that Levelrail's approach
    is not a drop-in replacement for that bundle: Swarm gives node
    membership, overlay networking, and a distributed scheduler as one
    package, whereas Levelrail deliberately keeps these separate:
    WireGuard mesh handles networking (this ADR), the custom reconciler
    handles orchestration (ADR 002), and placement stays manual-then-simple-
    spread with no bin-packing (Phase 3, per the project's non-goals). That's a
    conscious trade of Swarm's one bundled subsystem for two independently
    designed, individually simpler layers, not a claim of equivalent
    functionality with less code.
  - `prior-art-dokploy.md`'s own synthesis calls the networking/ingress
    issue cluster (#1802, #592, #3201, #821) "the single biggest pain
    cluster" in Dokploy's tracker, and names it direct validation for
    Levelrail's WireGuard-mesh decision specifically because multiple
    independent issue threads show the failure surfacing at the cross-node
    boundary, exactly where a Swarm-routing-mesh dependency would bite.

## Consequences

- The network abstraction has to be designed in Phase 1 as an interface,
  not a concrete WireGuard dependency, even though nothing implements the
  real mesh until Phase 3. Getting that interface wrong in Phase 1 means a
  Phase 3 rewrite of everything that depends on inter-node addressing, not
  just the networking package.
- Single-node deployments (the Phase 1 default) do not pay any WireGuard
  cost at all; the mesh only activates once a second node is added, which
  matches Phase 3's exit criterion of adding a second node from the UI.
- `wireguard-go` plus kernel-module detection is more code than depending
  on the in-kernel module unconditionally, but it's the same portability
  trade the project already makes elsewhere (pure-Go SQLite driver, ADR 007, for
  the same reason: cross-compilation and running on hosts you don't fully
  control shouldn't require a kernel feature you can't guarantee is
  present or loadable).
- Because this is Swarm's overlay network being replaced by a purpose-built
  mesh rather than Swarm's entire bundle being replaced piece for piece,
  Levelrail owns problems Swarm would otherwise have solved implicitly:
  peer key distribution and rotation, DNS resolution across the mesh, and
  what happens to WireGuard peer state when a node is cordoned or drained
  (Phase 3 work item). None of that is designed yet; this ADR fixes the
  approach, not the details.
