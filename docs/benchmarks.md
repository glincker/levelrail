---
title: "Load and failover benchmarks"
description: How to measure throughput, latency, ingress overhead and failover behavior under load, and which numbers are still unmeasured.
---

This page describes the harness and publishes the few numbers measured so far. Anything not in a dated, hardware-labeled row is marked not yet measured. [Performance](/performance) covers the idle footprint instead.

## What it measures

`levelrail-cli bench run` drives an app URL at a constant arrival rate and reports, client side:

- Requests per second achieved, and error rate (transport errors, timeouts and 5xx responses).
- Latency p50, p95, p99 and max.
- Ingress overhead: the same run against one replica directly (`--direct-url`), subtracted from the run through the ingress, per percentile.
- Per failure scenario: dropped requests, the error window (first to last failed request after the event), and recovery time (event to last failed request).

## Method

- Open loop. Request `i` is scheduled at `i / rate` seconds and launched on its own goroutine whether or not earlier requests have returned. A closed-loop generator slows down when the server stalls and so hides the stall (coordinated omission).
- Latency is measured from the intended send time, not from when the request left the client. A request that was due during a stall reports the queueing delay.
- The client caps in-flight requests at 20000. Requests over the cap are counted as errors and reported as `client_saturated`. A non-zero value means the load generator was the bottleneck and the run is not valid evidence about the server.
- Percentiles are nearest-rank over every request, failures included.
- A run is `recovered` when its last failure is more than one second before the end of the run. Pick a duration long enough to leave a clean tail.
- The generator should run on a different machine from the control plane, on the same network segment as a real client.

## Reproduce

Build the fixture app, deploy it with the replica count you want to test, and put it behind the [load balancer](/load-balancing) with a domain.

```bash
docker build -t bench-app test/fixtures/bench-app
```

The fixture answers `GET /` with `ok` and exposes `POST /__bench/die`, which makes that one container exit so the reconciler has to replace it.

Steady state, with the overhead comparison:

```bash
levelrail-cli bench run --url https://bench.example.com/ \
  --direct-url http://REPLICA_IP:8080/ \
  --rate 2000 --duration 60s --hardware "8 vCPU, 16 GB, kernel 6.8, Docker 27, 1 node" --json
```

Replica failover (stops one replica's container one third into the run):

```bash
levelrail-cli bench run --url https://bench.example.com/ --rate 500 --duration 90s \
  --kill-replica-url http://REPLICA_IP:8080/__bench/die
```

Node drain on a multi-node placement:

```bash
levelrail-cli bench run --url https://bench.example.com/ --rate 500 --duration 120s \
  --drain-node NODE_ID --drain-target OTHER_NODE_ID
```

Control plane restart (ADR 025: ingress should keep serving while the control plane is down):

```bash
levelrail-cli bench run --url https://bench.example.com/ --rate 500 --duration 90s \
  --restart-control-plane
```

The restart report includes `ingress_kept_serving`, true when no request failed after the restart was issued.

## Hardware disclosure

Publish these with every result, via `--hardware` and the report's `client` block:

| Field | Server | Load generator |
| --- | --- | --- |
| CPU model and core count | required | recorded automatically |
| RAM | required | n/a |
| Kernel version | required | n/a |
| Docker version | required | n/a |
| Node count and placement | required | n/a |
| Network between client and ingress | required | required |
| Replica count and app resource limits | required | n/a |

## Results

Only rows with a date and a hardware note are measured. Everything else is not yet measured, and no limit is claimed for it.

### Load driver against the fixture app, loopback (2026-10-18)

Hardware: Apple M4 Max, 14 cores, 36 GB RAM, macOS 27.2, Go 1.26.9. The fixture app (`test/fixtures/bench-app`, built from source, not in a container) and the load driver ran on the same machine over loopback, 15 seconds per rate, 1 replica, no ingress, no Docker. This shows the ceiling of the driver and the fixture, not of a Levelrail deployment: it is a client-side floor to subtract from real runs, not a production figure.

| Rate (req/s) | Achieved | Errors | p50 ms | p95 ms | p99 ms | Max ms | Client saturated |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 500 | 500 | 0 of 7500 | 0.56 | 3.77 | 9.57 | 26.6 | 0 |
| 2000 | 2000 | 0 of 30000 | 0.18 | 0.45 | 4.93 | 54.9 | 0 |
| 5000 | 5000 | 0 of 75000 | 0.16 | 6.16 | 16.45 | 40.7 | 0 |

Not yet measured: anything through the ingress, with Docker, across nodes or across zones.

| Scenario | Rate | Result | Status |
| --- | --- | --- | --- |
| Steady state through the ingress, requests per second achieved | not set | not yet measured | not yet measured |
| Steady state through the ingress, p50 / p95 / p99 latency | not set | not yet measured | not yet measured |
| Ingress overhead, p50 / p95 / p99 | not set | not yet measured | not yet measured |
| Zone loss: dropped requests, error window, recovery | not set | not yet measured | not yet measured |
| Replica stop: dropped requests, error window, recovery | not set | not yet measured | not yet measured |
| Node drain: dropped requests, error window, recovery | not set | not yet measured | not yet measured |
| Control plane restart: ingress kept serving, outage length | not set | not yet measured | not yet measured |

## What is tested without Docker

Zone-aware placement is tested against a fake fleet in `internal/placement`: replicas spread one per zone, and after losing a whole zone the replacement lands in a surviving zone. That test proves the scheduling decision, not request-level recovery, which is the unmeasured zone loss row above.

The load driver is unit tested against fake HTTP servers: arrival schedule, percentile math, error accounting, coordinated omission and failure-window analysis. Deploying the fixture and running the scenarios against real containers is not covered by those tests.
