# Multi-node: adding and managing nodes

Everything on this page is optional. A fresh install runs entirely on
the control plane's own local node, and nothing here has to be touched
for that to work: `node_id` on an app or database defaults to the empty
string, which means "this control plane's own node," and simple spread
placement only ever activates once a second node is actually
registered. Packages: `internal/api/nodes.go`, `internal/api/node_metrics.go`,
`internal/api/node_patch_status.go`, `internal/agent`,
`internal/reconcile/nodehealth`, `internal/reconcile/mesh`,
`internal/network`.

## Why a second node is optional, not assumed

The whole platform is designed single-node-first (see `CLAUDE.md`
section 1: "the target user runs between 3 and 50 services on between 1
and 10 machines"). A second node is something you add when one box runs
out of room, when you want to isolate builds from production
containers, or when you want a dedicated database host, not something
the platform makes you think about on day one.

Concretely, that shows up as:

- The dashboard's node picker (`NodeSelectField`, used by both the
  create-app and create-database forms) only renders once at least one
  node besides the local one exists. On a single-node install it never
  appears.
- Every placement field (`node_id` on an app or database) accepts the
  empty string as a real, permanent value meaning "the local node," not
  a placeholder for "not yet decided."
- Auto-placement (see below) only ever picks a remote node when one is
  registered, schedulable, and online; with none registered it leaves
  `node_id` empty, i.e. local, every time.

## Enrolling a second node

Enrollment is a one-time join token exchanged for a client certificate,
the "reverse-dialed gRPC agent with mTLS" design in `CLAUDE.md` section
4.3: the new machine's agent always dials out to the control plane, the
control plane never opens a connection to it.

1. **Mint a join token** (dashboard: "Add node" on the Nodes page, or
   the CLI):

   ```bash
   levelrail-cli nodes join-token
   ```

   This calls `POST /api/v1/nodes/join-tokens`, which mints a token
   valid for 15 minutes (`nodeJoinTokenTTL`, `internal/api/nodes.go`)
   and returns it in plaintext exactly once; the server only ever
   stores its hash, so there is no way to recover it later. If it
   expires unused, mint a new one, there is no renewal.

2. **Run the agent on the new machine**, pointing it at the control
   plane's agent gRPC listener (`:9443` by default, `APP_AGENT_ADDR` to
   override server-side):

   ```bash
   APP_CONTROL_PLANE_ADDR=controlplane.example.com:9443 \
   APP_JOIN_TOKEN=<token from step 1> \
   ./levelrail-agent
   ```

   `APP_NODE_NAME` is optional and defaults to the machine's own
   hostname. On first run the agent calls `DialEnroll`
   (`internal/agent/client.go`), which redeems the join token for a
   client certificate and a copy of the control plane's CA cert, then
   persists that identity to `./levelrail-agent-identity.json`
   (`APP_AGENT_IDENTITY_FILE` to move it, mode `0600` since it holds a
   private key). Every subsequent start of the same binary against the
   same identity file skips enrollment entirely and reconnects with the
   existing certificate; a join token is single-use
   (`MarkNodeJoinTokenUsed`) and only matters for that first run.

3. **Confirm it registered**:

   ```bash
   levelrail-cli nodes list
   ```

   There is no live "waiting for the agent" indicator in the dashboard
   dialog itself, by design (`AddNodeDialog.tsx`'s own note: "refresh
   this page once the agent connects"). The node appears in the list
   the moment `Server.Enroll` (`internal/agent/server.go`) saves it,
   with `status: pending` until its first heartbeat flips it to
   `online`.

4. **Once connected**, it is a normal placement target: pick it in the
   app or database create form's node picker, move an existing
   resource onto it (`PUT /api/v1/apps/{name}/node`, `PUT
   /api/v1/databases/{name}/node`, both outside this page's own scope),
   or let auto-placement send new resources there.

If the agent's local Docker daemon is reachable it serves every
container operation immediately; a node only accepts dispatched builds
once an operator explicitly turns `accepts_build_workloads` on (see
Workload capabilities below), even if its own BuildKit connected fine
at startup.

## Node health and heartbeat

A connected agent's session touches `last_seen_at` on an interval
(`APP_NODE_HEARTBEAT_INTERVAL`, default 15s,
`internal/agent.WithHeartbeatInterval`). A graceful disconnect (the
agent process exiting cleanly) flips the node to `offline`
synchronously. A hard kill or severed link does not: nothing ends the
gRPC stream cleanly, so nothing notices on its own. That is what
`internal/reconcile/nodehealth.Controller` exists for: every reconcile
pass re-reads `last_seen_at` fresh (never cached) and compares it
against `APP_NODE_HEARTBEAT_TIMEOUT` (default 45s). Past that timeout
with the node still marked `online`, the controller flips it to
`offline` itself and records a `Heartbeat` condition explaining why.

```bash
levelrail-cli nodes health <id>
```

reads that stored condition (`GET /api/v1/nodes/{id}/health`) plus a
live re-check of whether any node-scoped alert (patch status, disk
space, resource usage) is currently firing for that specific node. A
node that enrolled but has never connected shows `NeverConnected`, not
an error.

The control plane's own local node gets the identical treatment: it
just heartbeats itself (`localNodeHeartbeat`, `cmd/levelrail/mesh.go`)
rather than through a gRPC session, so it is never permanently `online`
by fiat either. `cordoned` is a defined `NodeStatus` value but nothing
in this codebase ever sets it: cordon is tracked as a separate boolean
field (`schedulable`), not folded into `status`, so a node can be
`online` and cordoned at the same time, or `offline` and schedulable
(pointless, but not contradictory).

## Cordon, drain, uncordon

Cordon marks a node unschedulable for new placements without touching
anything already running there. Drain is the separate, heavier action
that actually moves existing placements off.

```bash
levelrail-cli nodes cordon <id>
levelrail-cli nodes uncordon <id>
levelrail-cli nodes drain <id> [--target <node-id>]
```

- **Cordon/uncordon** (`POST /api/v1/nodes/{id}/cordon`,
  `.../uncordon`) are idempotent: cordoning an already-cordoned node is
  a no-op success. A cordoned node is excluded from auto-placement
  (`autoPlaceNode`, `internal/api/scheduling.go`) and refused as an
  explicit placement target (`validatePlacementTarget` returns a 400,
  not silently ignored).
- **Drain** (`POST /api/v1/nodes/{id}/drain?target_node_id=`) moves
  every service and database currently placed on the node to a target:
  an explicit `--target`, or, if omitted and auto-placement is enabled,
  each resource gets its own pick from the same least-loaded-node logic
  create uses, so draining spreads load across whatever else is
  registered instead of piling everything onto one node. This only
  changes desired placement immediately; the reconcile engine's next
  pass is what actually relocates each container. One resource failing
  to move does not stop the rest: the response is `200` on full success
  or `207 Multi-Status` when some resources failed, listing exactly
  what moved and what didn't (`drainNodeResponse`), never a bare 500 for
  a partial result.

Deleting a node (`DELETE /api/v1/nodes/{id}`, `nodes delete <id>` in the
CLI) is refused with `409` while it still has anything placed on it;
drain first. Delete is otherwise idempotent, deleting an
already-deleted node ID succeeds.

## Viewing what's placed on a node

There is no single "workloads on this node" list endpoint today.
What exists instead:

- **The dashboard's own app and database detail pages** show that
  resource's `node_id` directly (`AppOverview.tsx`, the equivalent
  database overview route), with a move-to-node control right next to
  it.
- **Drain's preview is really the closest thing to a placement list**:
  running a drain (or reading the two lists `handleDrainNode` and
  `handleDeleteNode` both consult server-side,
  `ListDesiredServicesByNode`/`ListDesiredDatabasesByNode`) is what
  actually enumerates everything on a node, which is why deleting a
  node with placements fails loudly rather than silently orphaning
  them.
- **The node's own metrics** (below) report a `resource_count`: how
  many placed services actually contributed a sample in the queried
  time range, which is a rough live signal of occupancy without being a
  real inventory.

A dedicated "list everything placed on node X" endpoint is real,
separate work; see Not built yet.

## Node-level metrics

```bash
levelrail-cli nodes metrics <id> --metric cpu_percent
```

`GET /api/v1/nodes/{id}/metrics` is deliberately not a host-monitoring
read. Read `handleQueryNodeMetrics`'s own doc comment
(`internal/api/node_metrics.go`) before assuming otherwise: for
`cpu_percent`, `memory_usage_bytes`, `network_rx_bytes`,
`network_tx_bytes`, `disk_read_bytes`, and `disk_write_bytes`, the
number returned is the *sum* of every placed app service's own
per-container samples (the same samples `internal/telemetry` already
collects per app), not a read of the host's real free/total memory or
CPU. `internal/agent` has no host-level stats collection today: no
`/proc` reads, no host-info gRPC message. `memory_limit_bytes` is
excluded entirely because an unconstrained container's reported limit
is approximately the whole host's memory, so summing it across N
containers would multiply that number by N rather than approach host
capacity.

Two metrics are real per-node host readings instead, not sums:
`disk_used_bytes` and `disk_total_bytes`, written by
`HostDiskCollector` directly under the node's own resource ID. The
dashboard's node metrics view (`NodeMetricsDashboard.tsx`) shows both
groups side by side, labeling the summed ones with a "summed across N
containers" subtitle so the distinction is visible, not just documented.

Databases placed on a node are excluded from the sum entirely (the
underlying query only walks `ListDesiredServicesByNode`, services, not
databases); folding them in is real, separate work.

## OS patch status

```bash
levelrail-cli nodes patch-status <id>
```

`GET /api/v1/nodes/{id}/patch-status` reads the latest sample
`HostPatchCollector` wrote for that node (interval:
`APP_OS_PATCH_CHECK_INTERVAL`, default 1 hour), searching back up to 48
hours (`osPatchLookback`) so a slow or just-restarted collector doesn't
lose the last real reading. The response distinguishes three states
that must never collapse into one: `checked: false` (no sample yet, no
supported package manager detected, or the collector hasn't run),
`checked: true, total: 0` (genuinely up to date), and `checked: true,
total: N` with a separate `security` count called out, since that's the
number worth acting on urgently. The dashboard's `NodePatchStatusCard`
renders exactly this three-way split as a single status badge, not a
chart, because it's one current fact, not a time series.

## Simple spread placement (auto-placement)

When you create an app or database and leave the node picker untouched
(or the dashboard's advanced panel is closed entirely, so `node_id`
never appears in the request body at all), the server decides
placement itself:

- If `APP_AUTO_PLACEMENT` is `false` (default: enabled), the new
  resource always lands on the local node, exactly the old behavior.
- Otherwise, `autoPlaceNode` (`internal/api/scheduling.go`) lists every
  registered node that is both `schedulable` and `online`, counts how
  many apps and databases each one already has, and picks the one with
  the fewest, tie-broken by the lexicographically smallest node ID for
  determinism. With no eligible node registered, it returns the local
  node, same as if auto-placement were off.

This is explicitly *not* bin-packing or resource-aware scheduling
(CLAUDE.md's own non-goals list rules that out for v1): it only counts
how many resources are already placed on a node, never CPU, memory, or
disk headroom. An explicit `node_id` (including an explicit empty
string, meaning "local, on purpose") always overrides this and is
validated against the same cordoned/unknown-node checks as every other
placement path.

The response for a create call that auto-placed carries
`auto_placed: true` alongside the `node_id` it picked, and the
dashboard surfaces that as a toast ("Auto-placed on node ... (simple
spread scheduling)") so it's never a silent decision.

## WireGuard mesh and internal DNS

This is the one piece on this page that is genuinely incomplete, not
just optional, and it's worth being direct about exactly how.

**Why it needs to exist at all**: an app's database connection string
is a literal value baked into a running container's environment.
Nothing rewrites it when the database moves to another node, so the
string has to be a name from the start, and the name has to resolve to
wherever the service currently lives. `internal/reconcile/mesh` is the
controller that keeps that mapping converged: every pass it reads every
node and placement fresh from the store, distributes WireGuard
configuration through `internal/network.Coordinator`, and rebuilds the
internal DNS zone (`<brand-short-name>.internal`, e.g. `levelrail`
gives `levelrail.internal`) from the same data.

**What actually works today**: the mesh is gated behind
`APP_MESH_ENABLED=1` (default off) and is non-fatal to misconfigure,
a control plane that would otherwise run fine still starts if the mesh
setup fails. Enabling it brings up this control plane's own WireGuard
device, its own DNS server, and a self-peer entry for its own node.

**What doesn't work yet**: `internal/network.ConfigSink`, the interface
that would carry a remote node's mesh config over the agent's gRPC
wire, is deliberately not built. Its only implementation right now is
`LocalSink`, which only ever configures the process it runs in. So
enabling `APP_MESH_ENABLED` on a control plane with a second, enrolled
node does not actually mesh that second node in: there is no message on
the agent wire contract yet to deliver its config, and no code path on
the agent side to apply one. It's real work, already scoped
(`ConfigSink`'s own doc comment names exactly what's missing: one new
op on the agent request/response contract, plus a case in
`internal/agent.Execute` that calls `Mesh.Apply`), just not landed.

**Even the single-node case has a real caveat**: the mesh DNS server
defaults to binding `:5390` (`APP_MESH_DNS_ADDR`), an unprivileged
port, because Docker's container DNS config and every container's own
resolver only accept a bare nameserver IP, never a custom port. That
means in the default configuration no container ever actually queries
this DNS server, full stop, verified against a real Docker daemon
during development: pointing a container at `<ip>:5390` fails the
container at start time with "bad nameserver address." To get any real
effect, an operator has to explicitly set `APP_MESH_DNS_ADDR` to bind
`:53` and grant the process permission to do so, a real operational
tradeoff this code does not paper over. Every failure short of that
(mesh disabled, DNS not on port 53, the bridge gateway lookup itself
failing) logs a warning and leaves containers using Docker's own
resolver exactly as before, never breaks anything.

There is no CLI or dashboard surface for mesh status today: nothing
here is operator-facing yet because there is nothing multi-node for an
operator to act on until `ConfigSink`'s gRPC arm lands.

## API reference

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/nodes` | `root` |
| `GET` | `/api/v1/nodes/{id}` | `root` |
| `DELETE` | `/api/v1/nodes/{id}` | `root` |
| `PUT` | `/api/v1/nodes/{id}/workloads` | `root` |
| `POST` | `/api/v1/nodes/join-tokens` | `root` |
| `GET` | `/api/v1/nodes/{id}/health` | `root` |
| `POST` | `/api/v1/nodes/{id}/cordon` | `root` |
| `POST` | `/api/v1/nodes/{id}/uncordon` | `root` |
| `POST` | `/api/v1/nodes/{id}/drain?target_node_id=` | `root` |
| `GET` | `/api/v1/nodes/{id}/metrics?metric=&from=&to=&step=` | `root` |
| `GET` | `/api/v1/nodes/{id}/patch-status` | `root` |

Every node route requires the `root` ability specifically, not `read`
or `write`: node management is treated as control-plane-level
administration, not per-resource access. `GET /api/v1/nodes/{id}` also
carries an `alert_status` field when telemetry is configured, a live
re-evaluation of that node's patch-status/disk-space/resource-usage
alert standing, not a stored value.

## CLI

```bash
levelrail-cli nodes list [flags]
levelrail-cli nodes get <id> [flags]
levelrail-cli nodes delete <id> [flags]
levelrail-cli nodes join-token [flags]
levelrail-cli nodes cordon <id> [flags]
levelrail-cli nodes uncordon <id> [flags]
levelrail-cli nodes drain <id> [--target <node-id>] [flags]
levelrail-cli nodes workloads <id> --accepts-app=BOOL --accepts-build=BOOL [flags]
levelrail-cli nodes health <id> [flags]
levelrail-cli nodes patch-status <id> [flags]
levelrail-cli nodes metrics <id> --metric NAME [--since DURATION | --from TIME --to TIME] [--step DURATION] [flags]
```

`nodes workloads` is a full replace of both flags, not a per-field
patch: both `--accepts-app` and `--accepts-build` are required on every
call, so an operator can't accidentally leave one unset and have it
silently reset to `false`. `accepts_build_workloads` is what actually
opts a node into dedicated build placement
(`internal/build.SelectBuildNode`); a new node accepts app workloads by
default but not build workloads, enabled explicitly, never implicitly
at enrollment.

## Not built yet (deliberate follow-ups)

- **The WireGuard mesh does not span nodes yet.** As covered above,
  `ConfigSink`'s gRPC arm (the wire contract change plus the
  agent-side `Mesh.Apply` call) is scoped but not built. Enabling
  `APP_MESH_ENABLED` today only wires up the control plane's own node.
- **No dedicated "what's placed on this node" endpoint.** The closest
  things today are drain's own resource enumeration and each resource's
  own `node_id` field on its detail page; there's no single list
  endpoint or dashboard panel that answers "show me everything running
  here" directly.
- **No resource-aware scheduling.** Auto-placement and drain's
  auto-spread both count placements, never CPU, memory, or disk
  headroom. Real bin-packing or affinity rules are an explicit
  CLAUDE.md non-goal for v1.
- **No dedicated build nodes routing yet beyond the capability flag.**
  `accepts_build_workloads` marks a node eligible; the actual dispatch
  logic living in `internal/build.SelectBuildNode` is real, but nothing
  here covers per-build placement policy beyond that boolean.
- **No node-scoped change history.** A cordon, drain, or workload
  toggle is captured by the platform's existing generic audit log
  (`GET /api/v1/audit-log`) like any other authenticated write; there's
  no node-specific history view beyond that.
