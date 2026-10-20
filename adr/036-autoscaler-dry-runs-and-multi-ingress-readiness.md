# ADR 036: Autoscaler contract, placement dry runs and multi-ingress readiness

Status: Accepted

Date: 2026-10-19

## Context

ADR 033 reversed the scheduler non-goal and ordered the work. Verifying the
repo before building showed most of steps 4 to 6 already exist
(`internal/placement`, `internal/autoscale`, zone spread, a decision history),
without an ADR recording the autoscaler's contract and without a way to ask
"what would happen" before it happens. Step 3 (several ingress nodes) had a
claim in `docs/resilience.md` but no test behind the certificate part.

## Decision

### Autoscaler contract (as built)

- `autoscale.Decide` is pure: no clock, no I/O. All inputs (time, current
  count, metric, last scale times, stabilization window state, spend cap,
  headroom) arrive in `Input`, so the same `Input` always gives the same
  `Decision`. Tests replay recorded traces.
- The count follows `ceil(current * value / target)` only outside a dead band
  (default scale up above target +10 percent, scale down below target -30
  percent), so a reading near the target moves nothing.
- Scale up is fast (60 s cooldown, step limit 4). Scale down is slow: a
  5 minute stabilization window that lands on the highest count recommended
  inside it, a 10 minute cooldown and a step limit of 1.
- Bounds always win: per-app min and max, then the operator's global ceiling
  (`APP_AUTOSCALE_MAX_REPLICAS`). Every refusal records a blocker reason
  (cooldown, stabilization, spend cap, no capacity, max, min, ceiling, step
  limit, no metrics) in the history shown on the app page, API and CLI.
- The controller writes only desired replicas. The reconciler converges them.
  Unknown metrics hold the count; they never scale.

### Placement dry runs

- `placement.Simulate` places N identical apps one after another against a copy
  of the fleet, each seeing the load the previous ones added, using the same
  `Spread` the real scheduler uses. Nothing is written.
- `placement.FailNodes` removes a zone or node from a copy of the fleet and
  reschedules its workloads onto survivors, largest memory request first then
  name, so the answer is deterministic. Workloads with no room are reported
  with the scheduler's own refusal reason.
- Both are read-only root routes (`GET /api/v1/nodes/placement-simulation`,
  `GET /api/v1/nodes/failure-simulation`), `levelrail-cli nodes simulate` and
  `nodes failure`, and a "What if" panel on the Nodes failure-domains card.
- The failure simulation answers the scheduling question only. It is not a
  measurement of request-level recovery, which stays an unmeasured row in
  `docs/benchmarks.md` until a multi-node chaos run exists.

### Multi-ingress readiness (design notes, not built)

- Certificate state is already shared through `ingress.SQLiteStorage` over the
  control plane database, including issuance locks with stale takeover. Two
  ingress processes pointed at the same database never hold one issuance lock
  together and see each other's certificates (`internal/ingress/certshare_test.go`).
- That sharing is by database file. A second ingress on another host needs
  either a replicated store (the open HA decision) or a certificate sync from
  the control plane to read-only ingress nodes. Recommended: the control plane
  stays the only issuer and pushes certificate bundles to ingress nodes over the
  existing agent channel, so ingress nodes never need database access.
- Failover between ingress nodes is a DNS or CDN concern (ADR 033 item 6):
  each ingress node exposes a health endpoint, the partner plugin removes an
  unhealthy node from DNS or the CDN origin pool. The control plane publishes
  the health; it does not steer.

## Rejected

- Forecasting or model-driven scaling: breaks reproducibility.
- Bin-packing in the simulator: the simulator mirrors the real scheduler,
  which is a spread and headroom scorer.
- Running the failure simulation by actually draining nodes: a dry run must
  have no effect.

## Consequences

Operators can check capacity and zone loss before an incident. The numbers are
only as good as the nodes' reported capacity; nodes that report none rank last
and the simulation says so through the unverified warning.
