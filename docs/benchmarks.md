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

### Control plane at 1000 apps (2026-10-18)

Hardware: Apple M4 Max, 14 cores, 36 GB RAM, macOS 27.2, Go 1.26.9, SQLite through modernc.org/sqlite in WAL mode on local SSD. The test seeds 1000 apps, 5000 deploy attempts and 1000 reconcile status rows into a fresh database, then times store calls in process, 200 samples each (50 for the full scans). This measures the database layer behind the list endpoints and the status writes of a reconcile pass. It does not include HTTP, JSON encoding, Docker, or the container work a real reconcile does, so treat it as a floor, not an end to end figure.

| Operation | p50 ms | p95 ms | p99 ms |
| --- | --- | --- | --- |
| List apps, page of 50 | 0.47 | 0.62 | 0.86 |
| List apps, all 1000 (no limit) | 4.82 | 5.06 | 5.58 |
| List apps, search `app-05`, page of 50 | 0.94 | 1.16 | 1.29 |
| Reconcile conditions, batch of 50 controllers | 0.07 | 0.09 | 0.20 |
| Latest deploy activity, all apps | 1.29 | 1.52 | 1.66 |
| Deploy attempts for one app | 0.03 | 0.04 | 0.19 |
| Reconcile status write sweep, 1000 controllers (5 sweeps) | 60.04 | 60.87 | 60.87 |

Seeding took 550 ms. Across the run the database logged 6353 statements, none over the 200 ms slow threshold, p95 0.38 ms. Reproduce with `LEVELRAIL_SCALE_BENCH=1 go test ./internal/store -run TestScaleSeeded -v`.

Not yet measured: anything through the ingress, with Docker, across nodes or across zones, and the HTTP list endpoints under concurrent load.

| Scenario | Rate | Result | Status |
| --- | --- | --- | --- |
| Steady state through the ingress, requests per second achieved | not set | not yet measured | not yet measured |
| Steady state through the ingress, p50 / p95 / p99 latency | not set | not yet measured | not yet measured |
| Ingress overhead, p50 / p95 / p99 | not set | not yet measured | not yet measured |
| Zone loss: dropped requests, error window, recovery | not set | not yet measured | not yet measured |
| Replica stop: dropped requests, error window, recovery | not set | not yet measured | not yet measured |
| Node drain: dropped requests, error window, recovery | not set | not yet measured | not yet measured |
| Control plane restart: ingress kept serving, outage length | not set | not yet measured | not yet measured |

## Chaos results

Measured by `levelrail-cli bench chaos run all` (build with `-tags chaoslive`) on 2026-10-18. Hardware: Apple M4 Max, 14 cores, 36 GB RAM, macOS 27.2, Docker Desktop engine 28.3.0 with 7 GiB visible to Docker, node:22-alpine fixture, one replica, 40 req/s open-loop for 30 s through the embedded ingress on loopback, one run per scenario. Single runs on a laptop: treat them as a first measurement, not a distribution.

| Scenario | Date | Status | Recovery | Failed requests | Error window | What happened |
| --- | --- | --- | --- | --- | --- | --- |
| kill-container | 2026-10-18 | passed | 3525 ms | 141 of 1200 | 3500 ms | `SIGKILL` of the only container, restarted in place by the reconciler, ingress routed again |
| daemon-unreachable | 2026-10-18 | passed | not applicable | 0 of 1200 | 0 ms | Docker API cut for 6 s mid-deploy, deploy resumed and succeeded, old container kept serving |
| control-plane-restart | 2026-10-18 | passed | 25 ms | 1 of 1200 | 0 ms | in-process engine and ingress restart, container not recreated, 0 direct probes failed |
| disk-pressure | 2026-10-18 | passed | not applicable | not applicable | not applicable | 10 MiB tmpfs scratch volume filled to 100%, node disk space rule fired, idle volume at 0% did not |
| oom-during-readiness | 2026-10-18 | passed | not applicable | 0 of 1200 | 0 ms | deploy failed as `OOMKilledDuringReadiness`, v1 kept serving |

How to read these:

- **daemon-unreachable** cuts the Docker API with a switchable socket proxy in front of the real daemon. The daemon itself was not stopped, so a daemon restart (engine events stream re-established, containers with restart policy `no`) is not measured. The first run found a real bug: while the Docker API was unreachable the ingress dropped every route (185 of 1200 requests failed for the length of the outage). The ingress now keeps each service's last known backend through an outage, and the run above is after that fix.
- **control-plane-restart** restarts the reconcile engine and embedded ingress inside one process. It does not exercise systemd socket activation, so the 25 ms figure is not the figure for a real `systemctl restart`. The 4 s outage in [resilience](/resilience) was measured on a server and still stands for that path.
- **disk-pressure** fills a size-limited tmpfs inside a throwaway container and feeds the measured usage to the real node disk space evaluator. It does not fill the host disk, so it proves the rule fires on the measured percentage, not that the host collector samples it.
- **kill-container** is slower than the 1 s once measured on a server (`docker kill`, 2 vCPU droplet) in [resilience](/resilience). They differ in hardware, Docker engine and whether the container was recreated or restarted in place, so neither replaces the other.

Not yet measured: daemon restart (real engine stop), disk pressure on the host volume with the collector in the loop, control plane restart under systemd socket activation, multi-replica kill, and everything across nodes or zones.

## Unified search

Measured by `go test ./internal/api -run TestSearch_ThousandAppsP95` on 2026-10-18. Hardware: Apple M4 Max, 14 cores, 36 GB RAM, macOS 27.2, SQLite on local SSD. 1000 apps, an admin token, 6 query shapes (exact name, action phrase, scoped, short term, broad term), 60 requests per run through the full HTTP stack including authentication and the per-hit authorization filter, three runs.

| Metric | Target | Measured |
| --- | --- | --- |
| p50 latency | not set | 10 to 11 ms |
| p95 latency | under 50 ms | 17 to 21 ms |

How to read this: the index holds roughly 8000 documents at 1000 apps (apps, domains, per-app actions, deployments, templates). Ranking looks at the best 500 candidates, so a very broad term costs the same as a narrow one. A caller with many IAM policies or team scoping pays more per hit than the admin token measured here, and that case is not yet measured.

## What is tested without Docker

Zone-aware placement is tested against a fake fleet in `internal/placement`: replicas spread one per zone, and after losing a whole zone the replacement lands in a surviving zone. That test proves the scheduling decision, not request-level recovery, which is the unmeasured zone loss row above.

The load driver is unit tested against fake HTTP servers: arrival schedule, percentile math, error accounting, coordinated omission and failure-window analysis. Deploying the fixture and running the scenarios against real containers is not covered by those tests.
