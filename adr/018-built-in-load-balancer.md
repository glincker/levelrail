# ADR 018: Built-in load balancer on the embedded Caddy pool

Status: Accepted

Date: 2026-09-24

## Context

`replicas: N` started N containers, but the ingress controller only ever
routed to replica 0. Operators wanted real balancing across replicas
(and across nodes) with health checks, sticky sessions and graceful
cutovers, and a way to reproduce the same shape on a cloud load balancer.

## Decision

Balancing is a thin layer over Caddy's `reverse_proxy` upstream pool,
which the embedded ingress (ADR 005) already drives.

- The model (`internal/loadbalancer`) is a per-service config stored as
  JSON in `service_load_balancers`. It maps onto Caddy's selection
  policies, active and passive health checks, retries, transport TLS and
  `stream_close_delay`.
- The ingress reconciler stays level-triggered: every pass rediscovers
  upstreams from the runtime (local or per-node through the agent
  registry), rebuilds the whole config and applies it. A pass that finds
  a partial pool still applies and reports `UpstreamsDegraded`.
- Live status is not stored. The reconciler records what it observed
  after a successful apply, and the API merges that with Caddy's admin
  `/reverse_proxy/upstreams` counters and an on-demand active probe.
- Slow start is done by the reconciler ramping the weights it hands to
  Caddy's `weighted_round_robin`, since Caddy has no native slow start.
- Export to Terraform, CDK, CloudFormation and Caddy is pure text
  generation from the same model, with warnings for settings the target
  cannot express.

## Rejected alternatives

- **Traefik or HAProxy sidecar.** A second proxy per app or per node
  duplicates TLS and routing config, breaks the single binary story
  (ADR 005), and adds a container to keep healthy. Caddy already has
  every policy needed.
- **Caddy dynamic upstreams (SRV or A lookups).** Replicas are Docker
  containers with host ports, not DNS records. Pushing the resolved list
  through the reconciler keeps one source of truth and works when a node
  is only reachable by mesh address.
- **Storing observed upstream state in SQLite.** It would add writes on
  every pass for data that is cheap to re-derive and stale within
  seconds.
- **Layer 4 balancing.** Out of scope; every route here is HTTP.
