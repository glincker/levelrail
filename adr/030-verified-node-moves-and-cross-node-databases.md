# ADR 030: Verified node moves and cross-node database connections

Status: Accepted

Date: 2026-10-10

## Context

Phase 3's exit criterion is: add a second server, move an app to it, and its
internal database connection keeps working; a drain moves everything off a
node. Before this change:

- A placement change flipped `node_id` and tore the source down at once, with
  no readiness check. Drain did the same for every resource, so a database
  landed on an empty volume on its new node.
- `move-with-volumes` ran in an in-memory goroutine: a control plane restart
  stranded it, and a failure left the app suspended.
- An app's `DATABASE_URL` used `<db>.<zone>:<engine port>` once mesh DNS was
  on, but a database was never published on the mesh, so the name resolved to
  an address with nothing listening. Without mesh DNS the URL used the
  container name, which only resolves on the database's own node.

## Decision

1. **Moves are a persisted state machine** (`internal/nodemove`, table
   `node_moves`): preflight, then for stateless apps place on target, verify,
   stop the old copy; for apps with volumes and databases stop the source,
   copy volumes (`backup.MoveVolume`), place, resume, verify. Every phase is
   idempotent and recorded before the next starts, so a restart resumes. A
   target that is not ready before `APP_NODE_MOVE_VERIFY_TIMEOUT` rolls back;
   the source data was only read, so resuming there restores it. A partial
   unique index allows one running move per resource.
2. **Readiness means fresh evidence from the target**: a `Ready=True`
   condition written after the placement change, plus a running container on
   the target node, so a stale report from the source never counts.
3. **Drain is a job of moves**: cordon, then databases before apps, one at a
   time, apps following their database's node when eligible. It stops at the
   first failed move and leaves the node cordoned.
4. **Cross-node databases get a stable mesh port.** When an app and its
   managed database end up on different nodes, the database gets a port from
   `APP_MESH_DB_PORT_MIN..MAX` and is published only on its node's mesh
   address. The app's URL becomes `<db>.<zone>:<mesh port>`, and the name is
   pinned in the container's `/etc/hosts` to the database node's mesh address,
   so it works without a DNS server. Same-node connections keep using the
   container name over the app's Docker network.
5. **Labels and selectors** filter placement; the scheduler spreads over
   eligible nodes by count (no bin-packing, per section 2).

## Consequences

- This reverses the earlier rule that a database is never mesh-bound. The new
  rule is narrower: only `internal/reconcile/database/mesh_endpoint.go` may do
  it, only for a database with an allocated port, and only on a validated
  unicast mesh address (a test enforces the confinement).
- A mesh-published database is reachable at the network level from every
  node in the mesh; it still requires its credentials. ADR 029's network scope
  is enforced when the app's connection is resolved, since no Docker network
  attachment happens across nodes.
- Without the mesh, a move that would split an app from its database is
  refused; inside a drain it is allowed briefly because the app follows.
- Allocating a mesh port restarts the database once to add the binding.

## Rejected alternatives

- **A forwarding sidecar per database**: avoids the restart, but adds an image
  and a hop for every query, and the obvious tool (socat) is GPL.
- **Running the mesh DNS server on every agent**: more moving parts than a
  hosts entry, and containers on remote nodes cannot reach the control
  plane's resolver address today.
- **Edge-triggered moves driven by the HTTP request**: what existed; it could
  not survive a restart.
