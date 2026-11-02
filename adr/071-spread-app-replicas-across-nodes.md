# ADR 071: Spread one app's replicas across nodes

Status: Proposed

Date: 2026-11-02

## Context

Levelrail runs several replicas of an app behind the built-in load
balancer, and a multi-node setup joins servers over the WireGuard mesh
(ADR 006). Every replica of one app still runs on one node. Read from
`release/v0.9.0`:

1. **One node per service.** `store.DesiredService.NodeID`
   (`internal/store/service.go`) is a single string, empty meaning the
   control plane's own node. `Replicas` is a count with no node per
   replica.
2. **One controller, one runtime.** `appControllersFor`
   (`cmd/levelrail/boot_source.go`) builds one `application.Controller`
   per service and resolves exactly one runtime from `svc.NodeID`. Inside
   the controller, readiness probes (`probeTarget`), liveness, mesh bind
   (`mesh_bind.go`), GPU checks, database link resolution
   (`mesh_database.go`) and the delete finalizer all read
   `desired.NodeID`.
3. **The ingress already speaks per-replica nodes.** `discoverUpstreams`
   (`internal/reconcile/ingress/loadbalancer.go`) resolves one runtime and
   host for the service through `NodeUpstreamResolver`, then inspects
   replica containers named by `ReplicaContainerName(svc, image, nonce, i)`.
   `loadbalancer.Upstream` already carries a `NodeID` per upstream. The
   implicit pool (`APP_INGRESS_IMPLICIT_LB`, on unless set false) gives
   round robin, 30s passive ejection and retries.
4. **Placement scores nodes, not replicas.** `internal/placement`
   (`Spread`, `Pinned`, zone, region and provider weights, `Simulate`)
   returns one node for a service.

So a node loss takes every replica of an app down at once, and adding a
node never adds capacity to an existing app.

## Decision

Add a per-replica node assignment, keep `NodeID` as the anchor, and run
one controller per (service, node) group. Behind a runtime setting that
defaults off.

1. **Store.** New column `replica_nodes` (JSON array, migration) on desired
   services, exposed as `DesiredService.ReplicaNodes []string`. Index `i`
   is the node for replica `i`. Empty means every replica on `NodeID`,
   which is every existing row, so upgrade changes nothing.
   `ReplicaNodes[0]` must equal `NodeID` when set.
2. **Assignment is persisted, not recomputed.** Placement computes the
   list at save time (deploy, scale, autoscale, drain) with a new
   `placement.SpreadReplicas` that applies replica anti-affinity on top of
   the existing scoring. Scaling up appends indices, scaling down removes
   the highest. A pass never moves a running replica.
3. **Reconciler.** `appControllersFor` groups indices by node and builds
   one controller per group with a projected `DesiredService`: `NodeID` set
   to the group node and a new option `WithReplicaIndices([]int)`. Every
   existing `desired.NodeID` reader keeps working unchanged because each
   controller sees one node. Container names keep the global index, so
   upstream IDs and slow start tracking stay stable. An unreachable node
   gets the existing node offline controller for its group only.
4. **Ingress.** `discoverUpstreams` resolves runtime and host per replica
   from `ReplicaNodes` and sets `Upstream.NodeID`. Remote dials go to the
   mesh address that `mesh_bind.go` already binds.
5. **Rollouts.** Rolling and recreate run group by group in index order,
   one group at a time, so the surge limit holds across nodes. Blue-green
   and canary stay single node in the first version and are refused with
   a reason when spread is on.
6. **Eligibility.** Spread is refused with a condition when the service
   has named volumes, app sleep, GPU requests or a layer4 stream. Those
   are node local today.
7. **Settings.** Runtime setting `apps.replica_spread` (bool, default
   `false`, env `APP_REPLICA_SPREAD` seeds it, applies live). Per app,
   `placement.spread: nodes` opts in. `apps.replica_spread_min_nodes`
   (int, default 2, env `APP_REPLICA_SPREAD_MIN_NODES`) is the eligible
   node count below which the app stays on one node and reports why.
   Turning the setting off stops new spread assignments; existing spread
   apps keep their list until redeployed.

## Alternatives rejected

- **One service per node (shards of an app).** Duplicates domains, env,
  links and history per shard; every API and dashboard view would need to
  merge them back.
- **One controller holding several runtimes.** The controller assumes one
  runtime in dozens of paths listed above. Rewriting them is larger and
  riskier than projecting one node per controller.
- **Delegating to an external orchestrator (Swarm or Kubernetes).** Adds a
  second scheduler, a second network model and hundreds of MB of footprint
  against a control plane that idles near 200 MB.
- **Document single-node replicas as the supported shape.** Honest and
  cheap, and it is what the docs must say until this ships, but it leaves
  node loss as a full outage for every app.

## Security

No new credentials or ports. Env, including secrets, already ships to
whichever node runs a service; spread sends it to more nodes, so the node
list is shown on the app page. Remote upstreams must bind to the mesh
address, never public; a replica whose mesh bind fails is held out of the
pool rather than falling back to a public bind.

## Failure modes

- **Node loss.** Its group goes offline, the pool ejects those dials, the
  remaining groups serve. The node failure policy (`store/node_failure.go`)
  reschedules only that group's indices in `auto` mode.
- **Mesh partition.** The control plane cannot inspect the remote group;
  upstreams report `inspect failed` and leave the pool. Capacity drops, the
  app stays up if any local replica is healthy.
- **Half-applied rollout.** A group fails readiness, later groups do not
  start, earlier groups keep the new release. The app reports a mixed
  release condition and rollback runs group by group.
- **Placement drift.** A node in the list is deleted. The list is
  repaired on the next save, not silently per pass.

## What cannot be verified locally

Cross-node latency, mesh MTU effects, node loss and partition behavior,
and drain under load. A laptop with one Docker daemon proves only the
store, projection and ingress JSON.

## Testing plan

Unit: four replicas over two labelled nodes give `[a, b, a, b]`; scale
down removes the highest index; projection gives each controller only its
indices; ingress emits four dials with the right node IDs; eligibility
refusals. Live Docker test with a fake second runtime for the group split.

A two-node test bed must prove, with dated rows in `docs/benchmarks.md`:
requests split across both nodes; a rolling deploy at steady load drops
no requests; stopping one node's agent drops requests only inside the
passive ejection window; auto reschedule restores the replica count; a
mesh partition leaves the app serving; drain moves one group without
touching the other.

## Slices

1. Store and placement: migration, `ReplicaNodes`, `SpreadReplicas`,
   runtime settings. Files: `internal/store/service.go`, new migration,
   `internal/placement/spread_replicas.go`,
   `internal/runtimesettings/registry_deploy.go`.
2. Reconciler grouping: `cmd/levelrail/boot_source.go`,
   `internal/reconcile/application/controller.go` (option only),
   `internal/reconcile/application/replica_names.go`.
3. Ingress per-replica nodes: `internal/reconcile/ingress/loadbalancer.go`,
   `internal/reconcile/ingress/remote_dial.go`.
4. Rollout ordering and refusals: `internal/deploy/rollout_policy.go`,
   `internal/reconcile/application/rollout_waves.go`.
5. API, CLI and dashboard placement card, then docs. Slice 2 and 4 touch
   the application reconciler and run one after the other.

## Consequences

Node loss stops being a full outage for stateless apps, and new nodes add
capacity to existing apps. The cost is a second dimension in every
reconciler pass and rollout, more nodes holding app secrets, and a
feature that stays off and undocumented in marketing until the two-node
rows exist.

## Open questions

1. Should blue-green and canary support spread in the first version or
   stay refused?
2. Is a per-app `placement.spread` opt-in needed, or should the runtime
   setting spread every eligible app?
3. Should the autoscaler place new replicas on the least loaded node or
   keep strict round robin across nodes?
4. Which two hosts form the test bed, and is the control plane one of them?
