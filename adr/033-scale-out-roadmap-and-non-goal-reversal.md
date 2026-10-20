# ADR 033: Scale-out roadmap and reversal of the scheduler non-goal

Status: Accepted

Date: 2026-10-17

## Context

The original scope named "a scheduler with bin-packing, affinity rules, or
autoscaling" as a non-goal for v1. That was right while the product had to
prove it could deploy one app well. It is no longer right. The vision now
has three horizons:

1. Replace the owner's Coolify instance and run every GLINCKER site on
   Levelrail.
2. Carry serious production traffic on a small footprint.
3. One free control plane over many clouds: EC2 boxes, cheap VPSes and home
   servers, each running the agent.

Horizon 2 and 3 need placement that understands load, scaling that reacts to
it, and failure domains. Without them the product stops at the size where
users outgrow Coolify, and that is where the gap in the market is.

A teardown of the hosted platforms and of Coolify and Dokploy showed the same
complaints repeatedly:

- Control plane outages that also take running workloads down.
- A single cloud or a single git host cascading into every customer.
- Autoscaling that is missing on self-hosted tools or paywalled on hosted
  ones.
- Surprise bills from bot traffic and egress.
- Mass automated actions with no dry run (a fraud model killing healthy
  services).
- Teams that find Kubernetes too heavy and Coolify-style tools too small, with
  nothing in between that scales across nodes without a platform team.

The pieces already in the repo are the base to build on: the level-triggered
reconciler (ADR 002), the reverse-dialed agent (ADR 003), the WireGuard mesh
(ADR 006), node-local observability (ADR 008), socket-activated ingress (ADR
025), `internal/placement`, `internal/loadbalancer` and
`internal/resourceadvisor`. None of them is yet measured under heavy load, and
the placement code is manual assignment, not scheduling.

## Decision

The non-goal is reversed. Resource-aware placement, failure-domain spread and
deterministic autoscaling are in scope, built in the order below. Bin-packing
and affinity rules stay out until a measured need appears.

1. **Measure first.** A load and failover harness (`levelrail-cli bench`) and a
   published limits table come before any scale claim. Request-level traffic
   metrics (rate, errors, p50/p95/p99 per app and replica) are core, not an
   add-on.
2. **Data plane independent of the control plane.** Established routes keep
   serving when the control plane is down, restarting or being upgraded. ADR
   025 removes the restart gap on one node. The next step is several ingress
   nodes sharing certificate state, and agents that keep the last good desired
   state and keep containers running with the control plane unreachable.
3. **Resource-aware placement.** New replicas go to a node chosen from measured
   free CPU, memory and disk, with the existing manual pin as an override. This
   is a scoring function over observed state, not a packing solver.
4. **Deterministic autoscaler.** A level-triggered controller reads a metric
   (CPU, memory, or request rate and latency per replica), compares it with a
   target band and writes a desired replica count within operator set minimum
   and maximum. Hysteresis is explicit: separate scale-up and scale-down
   thresholds, a cooldown window and a step limit, all configurable by
   environment and per app. The same inputs always give the same output. No
   model, forecast or AI is involved.
5. **Failure domains.** Nodes carry region, zone and provider labels. Spread
   places replicas of one app across distinct domains when capacity allows and
   reports a condition with a reason when it cannot. Replica routing prefers
   nearby healthy replicas using measured mesh latency.
6. **Geo steering through partners.** Global traffic steering (DNS, CDN,
   anycast, health based failover between regions) is delegated to providers
   such as Cloudflare or Akamai through plugins (ADR 034). The control plane
   publishes the health and weights; the partner does the steering.

### Delivery order and exit criteria

Each step gets its own ADR or design note before code, and ships to the
dashboard, CLI and API together.

| Step | Work | Exit criterion |
| --- | --- | --- |
| 1 | Bench harness and request-level metrics | `bench` runs against a real node and produces a repeatable report; numbers and limits are published |
| 2 | Data plane survives control plane loss | With the control plane stopped for 10 minutes, existing apps keep serving and containers keep running; recovery needs no operator action |
| 3 | Multi-ingress with shared certificate state | Two ingress nodes serve the same domain; killing one under load loses only in-flight requests |
| 4 | Resource-aware placement | New replicas land on the node with headroom in a test cluster; an override pin is honored; the chosen node and the reason are recorded |
| 5 | Failure-domain labels and spread | Replicas span at least two zones when available; losing a zone leaves the app serving; unsatisfiable spread shows a reason |
| 6 | Deterministic autoscaler | A step load raises replicas and removal of the load lowers them, with no oscillation inside the cooldown, reproducible from a recorded metric trace |
| 7 | Geo steering plugin | A partner plugin moves traffic away from an unhealthy region in a live test |

Step 1 gates every claim in the later steps. Step 7 depends on the plugin
contract in ADR 034 and does not block steps 1 to 6.

## Constraints preserved

- ADR 002: reconcilers stay idempotent, level-triggered and safe to
  interrupt. The autoscaler and the placer only write desired state or a
  placement decision; the existing reconciler converges it. Every decision
  emits a condition with a reason string.
- ADR 002: no edge triggers. A scale decision is recomputed from current
  observed state each pass, not from a remembered event.
- ADR 025: the single static binary and socket activation stay. Multi-ingress
  is added next to them, not by moving ingress out of the control plane
  binary.
- ADR 003 and ADR 032: agents dial out and enroll with their own keys. New
  node labels travel over the same channel, and no inbound port is opened on
  managed nodes.
- No AI in the reconciliation path. AI may explain a scaling decision or
  suggest settings, and may only act through the opt-in closed-catalog
  mechanism that the deterministic engine executes.
- Mass actions (drain a zone, scale many apps) have a dry run and a
  confirmation step.

## Rejected alternatives

- **Adopt Kubernetes or Nomad underneath.** Rejected: it brings the operating
  weight the target users are trying to avoid, a second source of desired
  state, and an ecosystem to track. It also discards the single binary and the
  Docker Engine API contract of ADR 002. We borrow patterns, not the runtime.
- **Bin-packing and affinity rules now.** Rejected: they are the largest part
  of a general scheduler and are only worth their complexity once measured
  fleets show waste that scoring cannot fix. Placement scoring plus spread
  covers the first generation of needs.
- **Build our own global anycast network.** Rejected: it needs presence in
  many networks and an operations team, which a free self-hosted project does
  not have. Partners already offer it and operators who need it often have an
  account there.
- **AI-driven scaling.** Rejected: non-deterministic decisions cannot be
  tested against a recorded trace, cannot be explained after an incident and
  would break the rule that the reconciliation path is deterministic.
- **Claiming Kubernetes parity before benchmarks.** Rejected: the category's
  reputation is set by its worst failure. The claim waits for published load
  and chaos results, and the limits table states what is not supported.

## Consequences

- The roadmap changes from breadth to proof: steps 1 and 2 come before any
  new scheduling feature.
- Autoscaling is free on owned nodes. There is no paywalled tier.
- Placement and spread add observed-state inputs (free resources, labels,
  latency) that the store and agent protocol must carry. That work needs
  migrations and a proto change, planned inside the steps that need them.
- Control plane high availability is still an open decision (SQLite
  replication or a Postgres driver). Until it is settled, "no single point of
  failure" is claimed only for the data plane, not the control plane.

## Risks

- A harness that measures a clean lab flatters the product. Mitigation: run it
  on small paid VPSes and publish the hardware, the method and the failures.
- Autoscaling flaps under noisy metrics. Mitigation: hysteresis and cooldown
  defaults, trace replay tests, and a per-app switch to pin replicas.
- Autoscaling can multiply spend. Mitigation: required maximum replicas, per
  app spend caps with alerts, and a visible reason on every change.
- Cross-cloud latency makes naive spread slower. Mitigation: spread is a
  preference ranked by measured latency, and the operator can restrict an app
  to one domain.
- Scope growth. Mitigation: each step has an exit criterion, and nothing moves
  to the next step on a promise.

## What we explicitly do not do

- No bin-packing, affinity or anti-affinity rule language yet.
- No predictive or learned autoscaling.
- No Kubernetes API, CRDs or operator compatibility.
- No self-built anycast or CDN.
- No managed cloud offering as part of this work.
- No public Kubernetes-alternative claim before the benchmark and chaos
  results back it.
