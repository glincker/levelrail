---
title: "Load and failover benchmarks"
description: How to measure throughput, latency, ingress overhead and failover behavior under load, and which numbers are still unmeasured.
"og:url": "https://docs.levelrail.com/benchmarks"
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

### Soak

`bench soak` churns read traffic against a control plane (a `levelrail demo` instance for a nightly run) and samples RSS, goroutines, open file descriptors and SQLite WAL size from the health endpoint. It fails when the end of the run is worse than the start by more than the limits, or worse than a ratchet file from the last passing run.

```bash
levelrail-cli bench soak --duration 24h --sample-interval 1m --csv soak.csv \
  --ratchet soak-ratchet.json --record
```

Limits default to 25 percent RSS growth, 100 goroutines, 50 descriptors, a 256 MiB WAL and 1 percent errors (`--max-*`, 0 disables one). It churns reads, not deploys, so it guards the API and store layers; deploy churn needs a Docker host.

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

### Control plane idle footprint (2026-10-19)

Live instance, 2 vCPU and 8 GB test box, all features at defaults, no apps under load: idle RSS 200 MB (peak 253 MB), 273 goroutines, 11 threads, 1.5 percent CPU, database p95 0.4 ms, reconcile pass 400 ms, metrics database 395 MB. The Control plane health page shows these numbers live, plus every background loop.

Boot harness (`go test ./cmd/levelrail -run TestFootprint`, 60 s idle per run after a 20 s settle). It starts the real control plane (`run()`: store, reconcile engine, telemetry, alerting, schedulers, ingress, agent server) on a temp data dir with no Docker daemon, once with every experimental feature off and once with all on, and records peak RSS, goroutines, threads, CPU and the number of loops on the health page. The Linux rows ran in a `golang:1.23` container limited to 2 CPUs and 8 GB from a cross-compiled test binary on an Apple M4 Max host; the binary carries the test runtime, so treat RSS as an upper bound for the shipped binary.

| Platform | Features | Goroutines | Threads | RSS MB | CPU percent | Loops on health page |
| --- | --- | --- | --- | --- | --- | --- |
| Linux arm64 | All experimental features off | 100 | 8 | 232 | 0.41 | 36 |
| Linux arm64 | All experimental features on | 104 | 8 | 233 | 0.40 | 38 |
| macOS arm64 | All experimental features off | 114 | not read | 170 | 0.22 | 36 |
| macOS arm64 | All experimental features on | 118 | not read | 170 | 0.52 | 38 |

Turning every experimental feature on costs 4 goroutines and 2 loops, and no measurable memory. The recorded baseline lives in `cmd/levelrail/testdata/footprint_baseline.json` and is a ratchet; see [Performance budget](/performance-budget). Live numbers on the 2 vCPU test box above are from the health page of a running instance, not this harness.

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
| Steady state through the ingress, requests per second achieved | up to 12800 req/s | 12800 req/s, 1 replica, trivial fixture, no ceiling found (2026-10-19, laptop) | measured, single run |
| Steady state through the ingress, p50 / p95 / p99 latency | not set | 0.66 / 1.82 / 9.51 ms at 12800 req/s (same run) | measured, single run |
| Ingress overhead, p50 / p95 / p99 | not set | not yet measured | not yet measured |
| Zone loss: dropped requests, error window, recovery | not set | not yet measured | not yet measured |
| Replica stop: dropped requests, error window, recovery | not set | see the 2026-10-22 run: 0 of 3000 failed, second replica back in 426 to 769 ms | measured, 3 runs, laptop |
| Node drain: dropped requests, error window, recovery | not set | not yet measured | not yet measured |
| Control plane restart: ingress kept serving, outage length | not set | not yet measured | not yet measured |

## Published run, 2026-10-22 (macOS laptop, not Linux production)

These are the first numbers produced with the results file, so each run carries its host, build and method. **They are not Linux production numbers.** Host: Apple M4 Max, 14 CPUs, 36 GiB RAM, macOS 27.2, Docker Desktop 28.3.0 with 7 GiB visible to Docker, so containers ran in a virtual machine. The load generator, the ingress and the fixture container (`node:22-alpine`, trivial response) shared the machine, over loopback with TLS. The host was busy with other work the whole time: 1 minute load average 13 to 30 on 14 CPUs at the start and end of the runs. Read every latency as a noisy upper bound and every throughput as a floor.

The raw files are in `docs/benchmark-results/`: `2026-10-22-all-scenarios.json` (every scenario, 2 runs each), `2026-10-22-deploy-rollout-concurrency.json` (the four scenarios that the first file skipped, 2 runs each) and `2026-10-22-replica-failover-rerun.json` (3 runs after the harness fix below). Each file records the commit of the harness that produced it. The `version` field in the first two files is a pseudo-version that Go stamped from a different checkout (the build was made inside a git worktree), so trust the `commit` field, which was set explicitly with `BENCH_COMMIT`.

What happened during the runs, because it changes how to read the rest:

- Docker Desktop stopped answering during or just after the second `ingress-steady` run, so the second round of `deploy-latency`, `rollout-under-traffic`, `ingress-concurrency` and `replica-failover` was skipped and re-run later from a restarted Docker Desktop. The first `ingress-steady` run ended with a 6400 req/s step that failed 34486 of 64000 requests. Whether the overload caused the Docker stop was not investigated.
- The first replica failover run set exposed a harness bug: when the container was replaced faster than the 200 ms poll, the restore time came out as a failure. The poll is now 50 ms, the dip is tracked explicitly, and a restore that was too fast to observe is reported as unresolved instead of failed. The three runs below are after that fix.

### Failover and recovery

Load is open loop at 40 req/s (single container) or 100 req/s (two replicas). Two runs per scenario unless stated.

| Event | Failed requests | Recovery or restore | Notes |
| --- | --- | --- | --- |
| `SIGKILL` of the only container | 142 of 1200, 147 of 1200 | healthy again after 3550 ms and 3675 ms (error windows 3525 and 3650 ms) | restarted in place by the reconciler; p95 about 2 s because requests queue during the outage |
| `SIGKILL` of 1 of 2 replicas | 0 of 3000 in all 3 runs | second replica running again after 426, 769 and 515 ms | request p99 10.6, 21.5 and 8.4 ms; restore time polled at 50 ms |
| Control plane restart (in process) | 8 of 1200, 3 of 1200 | ingress outage 200 ms and 75 ms | the app container was not touched; this is an in-process engine and ingress restart, not systemd |
| Docker API unreachable for 6 s during a deploy | 0 of 1200 in both runs | deploy resumed and succeeded | old container kept serving |
| OOM kill during the readiness wait | 0 of 1200 in both runs | deploy failed as `OOMKilledDuringReadiness` | version 1 kept serving |

### Deploy and rollout

| Measure | Result |
| --- | --- |
| Deploy latency, image to first answer from the new version | cold 4678, 3653 and 3442 ms over 3 runs; warm p50 3879, 4372 and 3578 ms and warm p95 4446, 4501 and 3922 ms (5 redeploys per run, so p95 is the maximum). All include the 3 s stability window the deploy gate waits for. |
| Rolling deploy of 2 replicas at 100 req/s, 40 s | 0 of 4000 failed in all 3 runs; p50 2.00, 2.56, 1.82 ms; p99 19.6, 25.4, 20.8 ms; max 46.7, 57.6, 319.5 ms |

### Request throughput through the ingress

Closed loop, one replica, 8 s per step, three runs (A, B, C). Requests per second, then p99 in ms.

| Concurrency | req/s A, B, C | p99 ms A, B, C | Errors A, B, C |
| --- | --- | --- | --- |
| 1 | 2007, 1393, 2220 | 3.1, 7.2, 1.7 | 0, 0, 0 |
| 8 | 6233, 8424, 9264 | 7.2, 5.1, 2.3 | 0, 0, 0 |
| 32 | 13911, 17236, 18538 | 9.2, 8.6, 4.9 | 0, 0, 0 |
| 128 | 10640, 6185, 6080 | 59.8, 174.5, 241.5 | 15, 58, 0 |

Open-loop ladder (`ingress-steady`, 10 s per step, stops at the first step over 1 percent errors or with client saturation): the highest clean step was 3200 req/s in the first run and 800 req/s in the second. In the second run the 400 and 800 steps already showed 13 and 11 errors and p95 over 800 ms, which is the busy host and a degrading Docker Desktop, not a property of the ingress. The 2026-10-19 run below reached 12800 req/s clean on a quieter moment; this run did not reproduce it. Treat the ceiling as unestablished and as strongly dependent on the host.

### Limits table for this run

| Question | Status |
| --- | --- |
| Deploy latency, rollout under traffic, replica kill, single container kill, control plane restart in process | measured, 2 to 3 runs, one noisy laptop, trivial fixture |
| Ingress throughput | measured at four concurrencies; the ceiling is not established (client and server share the machine) |
| Anything on a Linux server, a real network or a real application | not measured |
| Node agent link loss, node drain, zone loss | not measured: needs a second node |
| Control plane restart under systemd socket activation, or a real control plane crash | not measured: the restart is in process |
| Docker daemon restart | not measured: the API is cut with a proxy, the daemon keeps running |
| Statistical confidence | not measured: 2 to 3 runs per scenario, no confidence interval |
| Percentiles per app and replica in production | not measured here |

### Reproduce a published run

Build with `-tags chaoslive`, run on a machine with a local Docker daemon, and write the files:

```bash
go build -tags chaoslive -o levelrail-cli ./cmd/levelrail-cli
BENCH_NOISY_LOAD_RATIO=1 ./levelrail-cli bench chaos run all --repeat 3 \
  --out results.json --md-out results.md --hardware "short note on the machine"
```

`--out` writes the structured file (host CPU, memory, OS, kernel, Docker version and engine, load average at start and end, build version and commit, methodology, caveats, every run and its named metrics, and a limits table derived from what actually ran). `--md-out` renders the same data as Markdown. You can name several scenarios instead of `all`. `BENCH_NOISY_LOAD_RATIO` is the load per CPU above which the report adds a "host was busy" caveat; unset, the load is still recorded but never flagged. `BENCH_COMMIT` overrides the commit when a build from a git worktree is stamped with another checkout's commit. Skipped scenarios are kept in the file as skipped, never dropped. The named metrics are in the results file only, not in the control plane `--record` store.

### Store, show and compare runs

Add `--upload` to `bench chaos run` (with a signed-in CLI or `APP_API_TOKEN`) and the results file is stored on the control plane. Settings, Benchmarks lists the runs with host, commit and outcome, opens one with its caveats, per-scenario tables, throughput chart and limits table, and compares any two runs with a delta per metric.

```bash
levelrail-cli bench chaos run all --repeat 3 --out results.json --upload
levelrail-cli bench results list
levelrail-cli bench results show bench_1a2b3c4d5e6f
levelrail-cli bench results compare BASE_ID HEAD_ID
levelrail-cli bench results upload results.json
```

Each side of a comparison is the median over that side's runs. A change smaller than `APP_BENCHMARK_COMPARE_NOISE_PCT` percent (default 5) reads as the same, and the view warns when the two runs came from different platforms or hosts, a dirty tree or a single run. Uploading the same file twice stores it once. `APP_BENCHMARK_RUNS_KEEP` (default 50) bounds how many runs are kept, oldest first. The API is `GET/POST /api/v1/benchmarks`, `GET /api/v1/benchmarks/{id}`, `GET /api/v1/benchmarks/compare?base=&head=` and `DELETE /api/v1/benchmarks/{id}` (root). The MCP tool `get_resilience_report` lists the stored runs, and takes `run_id` and `compare_to`.

The dashboard cannot start a scenario. The scenarios build their own throwaway stack on a local Docker daemon and need a CLI built with `-tags chaoslive`, so running them inside the control plane would put test load and container churn next to production apps.

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

### Steady load through the embedded ingress (2026-10-19)

Measured by `levelrail-cli bench chaos run ingress-steady` (build with `-tags chaoslive`) on 2026-10-19. Hardware: Apple M4 Max, 14 cores, 36 GB RAM, macOS 27.2, Docker Desktop engine 28.3.0 with 7 GiB visible to Docker, node:22-alpine trivial fixture (returns a short string), one replica, TLS through the embedded ingress on loopback, open-loop constant arrival rate, 10 s per step, one run. The load driver and the stack share this laptop, so this is a floor for the driver plus ingress plus one trivial container, not a server figure.

| Rate (req/s) | Achieved | Errors | p50 ms | p95 ms | p99 ms | Max ms |
| --- | --- | --- | --- | --- | --- | --- |
| 100 | 100 | 0 of 1000 | 2.68 | 3.11 | 3.73 | 7.4 |
| 400 | 400 | 0 of 4000 | 1.08 | 1.70 | 2.69 | 19.8 |
| 800 | 800 | 0 of 8000 | 0.77 | 1.17 | 2.21 | 8.5 |
| 1600 | 1600 | 0 of 16000 | 0.46 | 0.65 | 1.11 | 6.2 |
| 3200 | 3200 | 0 of 32000 | 0.31 | 0.56 | 1.20 | 9.0 |
| 6400 | 6398 | 19 of 64000 | 0.45 | 1.59 | 10.31 | 68.4 |
| 12800 | 12800 | 0 of 128000 | 0.66 | 1.82 | 9.51 | 42.3 |

No ceiling was reached: the top step ran with no client saturation, so the limit for this machine is above 12800 req/s for this fixture, and the run stopped there. The 19 errors at 6400 (0.03 percent) were not reproduced in the step above it, and were not investigated. Lower rates show higher latency because the first requests pay for TLS and connection setup; do not read that as the ingress getting faster under load. Ingress overhead against a direct path, several replicas, several apps and a real application are not measured here.

Not yet measured: daemon restart (real engine stop), disk pressure on the host volume with the collector in the loop, control plane restart under systemd socket activation, multi-replica kill, and everything across nodes or zones.

## Deploy, rollout, concurrency and failover (2026-10-19)

Measured by `levelrail-cli bench chaos run <scenario> --json` (build with `-tags chaoslive`; add `--markdown` for a table). Scenarios: `deploy-latency`, `rollout-under-traffic`, `ingress-concurrency`, `replica-failover`. Each builds its own local control plane slice (reconcile engine, embedded ingress, store) on a real local Docker engine and never touches a remote server. Hardware: Apple M4 Max, 14 cores, 36 GB RAM, macOS 27.2, Docker Desktop engine 28.3.0, Go 1.26.9, node:22-alpine trivial fixture, TLS through the embedded ingress on loopback, load driver on the same machine. The machine was busy with other builds (load average about 30 of 14 cores), so read every figure as a noisy upper bound on latency and a lower bound on throughput. One run per row unless stated.

| Scenario | Result |
| --- | --- |
| Deploy latency, image to first answer from the new version | cold deploy 4.9 s; 5 redeploys p50 4.26 s, p95 5.53 s, min 3.65 s, max 5.53 s (includes the 3 s stability window the gate waits for; n=5, so p95 is the maximum) |
| Rolling deploy of 2 replicas at 100 req/s open loop, 40 s | 0 of 4000 requests failed; p50 2.40 ms, p95 12.99 ms, p99 24.29 ms, max 80.6 ms; a probe saw both v1 and v2 answering during the run |
| Kill 1 of 2 replicas at 100 req/s, 30 s | 0 of 3000 requests failed, error window 0 ms; second container running again 692 ms after the kill (200 ms poll) |
| Ingress, closed loop, 1 replica, 8 s per step | see the concurrency table |

Ingress at fixed concurrency, two runs (the first run's c32 reached 13512 req/s on a quieter moment, shown for the spread):

| Concurrency | Run A req/s | p50 / p95 / p99 ms | Run B req/s | p50 / p95 / p99 ms | Errors |
| --- | --- | --- | --- | --- | --- |
| 1 | 1880 | 0.38 / 1.19 / 2.69 | 813 | 0.95 / 2.74 / 6.64 | 0 |
| 8 | 4352 | 1.37 / 4.54 / 7.88 | 3652 | 1.60 / 5.50 / 10.84 | 0 |
| 32 | 9982 | 2.55 / 7.03 / 13.99 | 4320 | 5.47 / 16.85 / 43.33 | 0 |
| 128 | 4636 | 11.21 / 102.64 / 387.39 | 4537 | 11.68 / 109.27 / 305.49 | 0 and 130 of 36665 |

The 128 step is slower than 32 because client and server share this laptop; it is not an ingress limit.

### Idle footprint, 0, 10 and 50 apps

`BENCH_COUNTS="0 10 50" BENCH_SECONDS=30 scripts/bench-idle.sh`, dev-mode control plane, apps created then stopped (no containers). CPU 0.10, 0.60, 0.37 percent. RSS read 248, 128 and 84 MB, which is not monotonic: it reflects Go garbage collection timing after startup, not a footprint that shrinks with apps. API p95 stayed under 8 ms for the list, status and certificate endpoints at 50 apps (apps list p50 2.16 ms, p95 4.79 ms). Running containers are not included, so this is a floor for the control plane, not a fleet figure.

### Limits table

| Question | Status |
| --- | --- |
| Deploy latency, rollout under traffic, replica kill, ingress throughput on one laptop, trivial app | measured, single or double runs, noisy host |
| Per app and per replica p50, p95, p99 | exists: `GET /api/v1/apps/{name}/traffic` and its CLI client report per upstream latency percentiles; not re-measured here |
| Node agent link loss, drain, zone loss | not measured: needs a second node, no local equivalent |
| Anything on a real server, real network or a real application | not measured |
| Statistical confidence (more than 6 deploys, repeated runs, quiet host) | not measured |
| Idle footprint with running containers, or at 100+ live apps | not measured by this harness |
| Ingress ceiling | not found; the client was the limit at c128 |

Dashboard: no page. This harness is an operator CLI plus these docs, a deliberate exception to the UI-with-every-feature rule because the output is a published measurement, not a control.

## Unified search

Measured by `go test ./internal/api -run TestSearch_ThousandAppsP95` on 2026-10-18. Hardware: Apple M4 Max, 14 cores, 36 GB RAM, macOS 27.2, SQLite on local SSD. 1000 apps, an admin token, 6 query shapes (exact name, action phrase, scoped, short term, broad term), 60 requests per run through the full HTTP stack including authentication and the per-hit authorization filter, three runs.

| Metric | Target | Measured |
| --- | --- | --- |
| p50 latency | not set | 10 to 11 ms |
| p95 latency | under 50 ms | 17 to 21 ms |

How to read this: the index holds roughly 8000 documents at 1000 apps (apps, domains, per-app actions, deployments, templates). Ranking looks at the best 500 candidates, so a very broad term costs the same as a narrow one.

### Scoped caller

Measured by `go test ./internal/api -run TestSearch_ScopedCallerP95` on 2026-10-19, same hardware, with other builds running on the machine (load average near 30, so read these as upper bounds). The caller is a developer in the default team (team scope active) holding 50 IAM deny policies on individual apps, using a session cookie. 6 query shapes, 30 requests per row, p50 and p95 in milliseconds.

| Apps | Endpoint | p50 | p95 |
| --- | --- | --- | --- |
| 1000 | `/api/v1/search` | 9 | 17 |
| 1000 | `/api/v1/apps?q=` (ranked, page of 50) | 12 | 15 |
| 5000 | `/api/v1/search` | 19 | 49 |
| 5000 | `/api/v1/apps?q=` (ranked, page of 50) | 31 | 37 |

The first run of this case measured 31 ms p50 and 50 ms p95 at 1000 apps for search, over the budget, because every authorization check re-parsed all 50 policy documents. Parsed documents are now cached by their text, which removed the cost without a per-principal visible-set cache. The ranked apps list pages over names first and loads only the page's rows.

## What is tested without Docker

Zone-aware placement is tested against a fake fleet in `internal/placement`: replicas spread one per zone, and after losing a whole zone the replacement lands in a surviving zone. That test proves the scheduling decision, not request-level recovery, which is the unmeasured zone loss row above. The same code backs `levelrail-cli nodes failure`, a dry run of that decision on your fleet (see [Multi-node](/multi-node)).

Certificate sharing between two ingress processes on one database is tested in `internal/ingress/certshare_test.go`: one stores, the other loads, and racing issuance locks never overlap. Ingress nodes on separate hosts are a design note in ADR 036, not built.

The load driver is unit tested against fake HTTP servers: arrival schedule, percentile math, error accounting, coordinated omission and failure-window analysis. Deploying the fixture and running the scenarios against real containers is not covered by those tests.
