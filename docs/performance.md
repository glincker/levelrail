---
title: "Performance"
description: Measured idle footprint of the control plane at 0, 100, and 500 apps, and how to reproduce it.
"og:url": "https://docs.levelrail.com/performance"
---

## Method

`scripts/bench-idle.sh` builds the control plane, boots it in dev mode in a
throwaway data directory, and for each app count N:

1. Creates apps up to N through the HTTP API and stops each one, so every app
   is suspended. No containers run and no images are pulled.
2. Waits 5 seconds, then samples the process's cumulative CPU time across a
   60 second idle window and reads RSS at the end.
3. Sends 200 sequential GET requests per endpoint over one connection and
   reports p50 and p95 latency in milliseconds.

## Results

Apple M4 Max (14 cores), macOS, dev build, same-machine loopback client.

| Apps | RSS (MB) | CPU (%) |
| --- | --- | --- |
| 0 | 67.7 | 0.03 |
| 100 | 82.6 | 0.30 |
| 500 | 90.8 | 1.60 |

Latency in ms, p50 / p95:

| Endpoint | 0 apps | 100 apps | 500 apps |
| --- | --- | --- | --- |
| `GET /apps` | 0.14 / 0.21 | 0.94 / 1.41 | 3.69 / 4.59 |
| `GET /apps/{name}` | n/a | 0.18 / 0.43 | 0.15 / 0.20 |
| `GET /system/status` | 0.70 / 1.32 | 0.66 / 0.85 | 0.53 / 0.83 |
| `GET /system/doctor` | 231.52 / 459.98 | 222.72 / 403.57 | 217.19 / 450.71 |
| `GET /certificates` | 0.11 / 0.21 | 0.14 / 0.24 | 0.14 / 0.23 |
| `GET /deploys/failed?since=24h` | 0.12 / 0.18 | 0.12 / 0.20 | 0.32 / 0.36 |

`GET /system/status` caches Docker disk usage for 30 seconds (errors are never cached), so it does not pay the roughly 370 ms `docker system df` cost on every request. `GET /system/doctor` takes about 220 ms because it runs live checks (Docker, database, and others) by design, and it is not on any polling path.

## Server profiles

`TestFootprint` (`cmd/levelrail/footprint_test.go`) boots the real control plane, with no Docker daemon and no apps, once per server profile (see [Server profiles](/profiles)), waits 20 seconds to settle, then idles for the measuring window and counts goroutines and registered background loops. Machine: Apple M4 Max, 14 cores, 36 GiB RAM, macOS 27.2, Go 1.26.9, dev build, 20 second window, one run per row (2026-10-19).

| Profile | Goroutines | Background loops | Idle RSS | Idle CPU |
| --- | --- | --- | --- | --- |
| `minimal` | 116 | 35 | 162.0 MB | 0.13% |
| `standard` | 118 | 38 | 162.7 MB | 0.13% |
| `suite` | 128 | 41 | 166.5 MB | 0.15% |

Read this honestly:

- `minimal` runs 2 fewer goroutines and 3 fewer loops than `standard`, and `suite` runs 10 more goroutines and 3 more loops. The test fails if `minimal` ever stops being lighter than `standard`.
- RSS and CPU are inside the run to run variance (10 to 15%). A profile does not make the process smaller, it makes less of it run.
- The savings the profile is for do not show up as goroutines: a shorter retention, no listener on a public port, no mesh or plugin runtime, and fewer warnings. The pipeline engine now backs off to a one minute poll with no active run, which cuts wakeups from 30 per minute to 1 but is the same goroutine count.
- The Linux baseline in `cmd/levelrail/testdata/footprint_baseline.json` has no profile rows yet. Record them on a Linux host with `FOOTPRINT_RECORD=1`.

Reproduce it: `APP_FOOTPRINT_IDLE_SECONDS=20 go test ./cmd/levelrail -run '^TestFootprint$' -v -count=1`. It takes about four minutes and is skipped under `-short`.

## Idle memory: where it goes

Live heap at idle is only about 20 MB. Most of the resident memory was transient boot garbage the Go runtime kept: the template catalog compose parse (about 51 MB allocated), password hashing set up at boot (64 MB, inside the auth library), and the first Docker Hub tag refresh (about 27 MB, one minute after start). Changes: the template catalog flags are computed on first catalog read, the first tag refresh waits `APP_DBUPGRADE_REFRESH_FIRST_DELAY` (default 10m), GC runs at `APP_GC_PERCENT` (default 50, ignored when `GOGC` is set), and a `memory trim` loop returns idle heap to the OS every `APP_MEM_TRIM_INTERVAL` (default 2m, `0` disables) when more than `APP_MEM_TRIM_MIN_IDLE_MB` (default 24) is unreleased.

One run each on an Apple M4 Max (macOS 27.2, Docker not reachable, dev mode with fixtures, 135 seconds after start, nothing else running). Footprint is macOS `footprint` (physical), the figure to trust on macOS; `ps` RSS is not, because macOS keeps freed pages resident until memory pressure.

| | Before | After |
| --- | --- | --- |
| Physical footprint | 90 MB | 71 MB |
| Heap in use | 29.9 MB | 28.6 MB |
| Heap reserved from OS | 169 MB | 127 MB |
| Goroutines | 153 | 155 |

Wakeups per minute were not measured beyond `top` showing 0 idle wakeups over three 10 second samples in both runs. The new trim loop adds one wake every two minutes. Linux numbers are not measured here.

Repeat it: build `./cmd/levelrail`, run it with `APP_PPROF_ADDR=127.0.0.1:<port>` (loopback only, off by default) plus the usual dev env and a private `APP_DATA_DIR`, wait two minutes, then read `/debug/pprof/heap?gc=1` and `/debug/pprof/allocs` with `go tool pprof -sample_index=alloc_space -top -cum`, and `footprint -p <pid>`.

## Linux container idle footprint

`scripts/idle-footprint.sh` cross-builds the control plane for the Docker server's architecture, starts it in a clean `debian:bookworm-slim` container, creates N apps through the API, waits for the settle period, then samples the process from `/proc` over the window. Per app count it boots a fresh process and data directory. It prints JSON with RSS (end and peak), CPU percent over the window, goroutines, open file descriptors, threads and background loop count.

**These are container-on-Docker-Desktop numbers, not bare metal.** The container runs in the Docker Desktop Linux VM on macOS (Docker 28.3.0, 14 CPUs, 8 GiB RAM, linux/arm64 on an Apple M4 Max). Memory and CPU accounting inside that VM differ from a native Linux host, so treat them as a Linux data point and not as a VPS figure. A bare metal or cloud VM run is still open.

One run per row, 60 second window after a 20 second settle, dev mode build (the dev token fixtures are how the script authenticates), apps created and then stopped so no app containers exist, and the Docker socket is not mounted, so the control plane runs with no reachable Docker daemon.

| Apps | RSS at end (MB) | Peak RSS (MB) | CPU (%) | Goroutines | Open fds |
| --- | --- | --- | --- | --- | --- |
| 0 | 212.0 | 212.0 | 0.38 | 129 | 27 |
| 5 | 216.1 | 216.1 | 0.60 | 124 | 27 |
| 20 | 148.0 | 233.2 | 0.83 | 127 | 29 |

Read this honestly:

- The 20 app row dropped from a 233 MB peak to 148 MB at the end because the memory trim loop returned boot garbage to the OS mid-window. The guard compares peak RSS for that reason.
- Idle CPU rose from 0.38% to 0.83% between 0 and 20 stopped apps in these runs. That is more than the sub-1% target implies for 20 trivial apps and it is one run per row, so the trend is real enough to chase but the slope is not precise. Per loop accounting (`IDLE_LOOPS_DUMP=1`) over a separate 60 second run showed the `reconcile pass` loop going from 110 ms to 184 ms of busy time for two passes at 0 versus 20 apps (about 3.7 ms of work per app per pass) and `diskguard` from 36 ms to 123 ms for one run. The remaining rise is not attributed, because most background tickers are not registered with the loop tracker.
- Goroutines and file descriptors did not grow with stopped apps.
- With running apps (`IDLE_APP_MODE=running`, which mounts the host Docker socket and starts real `traefik/whoami` containers) only a 15 second, 2 app smoke run completed (224 MB, 182 goroutines, 0.40% CPU). That is too short to publish as a figure. The full 0, 5 and 20 running app run did not finish because the Docker daemon on the test host stopped responding, so there is no committed running app baseline.
- The separate node agent was not measured: the control plane serves the local node in process, which is what this harness runs.

Reproduce it:

```bash
scripts/idle-footprint.sh                        # 0, 5 and 20 stopped apps, JSON on stdout
IDLE_COUNTS=0,50 IDLE_WINDOW_SECONDS=120 scripts/idle-footprint.sh
IDLE_APP_MODE=running scripts/idle-footprint.sh  # real containers via the host Docker socket
```

Knobs: `IDLE_COUNTS`, `IDLE_WINDOW_SECONDS`, `IDLE_SETTLE_SECONDS`, `IDLE_APP_MODE` (`stopped` or `running`), `IDLE_APP_IMAGE`, `IDLE_IMAGE`, `IDLE_ARCH` and `IDLE_LOOPS_DUMP=1`.

### Regression guard

The committed baseline is `scripts/idle-footprint-baseline.json`, keyed by OS, architecture, app count and app mode. The guard is off unless you ask for it, and it has no built in tolerances: every limit comes from the environment, and a missing one is an error.

```bash
IDLE_FOOTPRINT_CHECK=1 IDLE_TOL_RSS_PCT=20 IDLE_TOL_GOROUTINES=15 \
  IDLE_TOL_FDS=10 IDLE_TOL_CPU_POINTS=0.5 scripts/idle-footprint.sh
```

`IDLE_TOL_RSS_PCT` is a percentage over baseline peak RSS, `IDLE_TOL_GOROUTINES` and `IDLE_TOL_FDS` are absolute counts, and `IDLE_TOL_CPU_POINTS` is absolute percentage points. Run `IDLE_FOOTPRINT_RECORD=1 scripts/idle-footprint.sh` to merge a run into the baseline, on the same kind of host the check will run on. A baseline recorded on Docker Desktop will not fit a bare metal CI runner, so record one there before enabling the check in CI. The comparison logic has unit tests in `scripts/idle-footprint`.

### Live footprint

The same baseline is compiled into the control plane, so a running instance can judge itself. Settings, Control plane health shows a Footprint card with resident memory, CPU averaged over the last `APP_FOOTPRINT_CPU_WINDOW_SECONDS` (default 60), goroutines and open file descriptors, each next to the baseline row for this OS and architecture with the nearest app count and the limit the `IDLE_TOL_*` variables allow. Memory is compared with the baseline peak. `GET /api/v1/system/footprint` returns the same assessment, `system health` prints it, and the `idle_footprint` doctor check (also served by the MCP tool `get_system_doctor`) warns when a metric is over its limit.

The status is `tolerance_unset` until all four `IDLE_TOL_*` variables are set on the control plane, and `no_baseline` on a platform with no committed row (for example macOS). The node agent's footprint is not measured by this card.

## Caveats

- One machine, one run per configuration. Expect run-to-run variance of roughly 10 to 15% on RSS and CPU.
- No containers are running. Real apps add Docker event, health probe, and
  metrics collection load that this benchmark does not measure.
- Dev build in dev mode, not a release binary. Loopback client, so no network
  latency is included.
- Apps are suspended, which exercises the reconcile resync path but not
  running-app reconciliation.

## Benchmark script notes

- The default API rate limits throttle a 200-request burst, so the script raises `APP_API_RATE_LIMIT_READ_RPM` and `APP_API_RATE_LIMIT_WRITE_RPM`.
- The brand file is resolved relative to the working directory. Run the script from the repo root (it does), or set `APP_BRAND_FILE` when running the binary from elsewhere.

## Re-running

`scripts/bench-idle.sh`

The defaults are N = 0, 100, 500 and a 60 second window. For a quicker run:

```bash
BENCH_COUNTS="0 100" BENCH_SECONDS=20 scripts/bench-idle.sh
```

Other knobs: `BENCH_REQUESTS` (requests per endpoint, default 200) and
`BENCH_PORT`. It needs bash, curl, ps, awk, and the Go toolchain, and cleans up
its data directory on exit.

## Dashboard at scale

`scripts/perf-measure.sh` seeds a scratch data dir with 1000 apps, 200 databases, 50 nodes and 20,000 deploy rows (`scripts/perf-fixtures`, deterministic, refuses a used data dir), runs a local control plane on it with Docker unreachable, and loads each page cold in headless Chromium through `scripts/perf-measure.py`. Everything is deleted on exit and nothing leaves the machine.

- Time to interactive is the later of network idle and the last long task end. Long tasks and blocking time come from the Long Tasks API.
- The CPU is throttled 4x (`PERF_CPU_THROTTLE`) so main thread work shows up on a fast laptop. Values are the median of 5 cold loads after one warm-up.
- API KB and JS KB are decoded sizes. The production build also serves `.br` and `.gz` copies of static assets, which this does not credit.
- Machine: Apple M4 Max, macOS 27.2, one run at a time. Treat the numbers as comparable between runs on one machine, not across machines. Deploy and request counts are the fixture's, not a real fleet's.

Before and after the change that lazy-loads the dock terminal and fleet chart and pages `/deploys/failed`:

| Page | TTI before | TTI after | Long tasks | API KB | JS KB before | JS KB after |
|---|---|---|---|---|---|---|
| Dashboard home | 2262 ms | 2056 ms | 6 to 4 | 692 to 693 | 5835 | 5090 |
| Apps list | 1944 ms | 1548 ms | 5 to 4 | 472 | 2172 | 1748 |
| Deploys list | 1904 ms | 1479 ms | 5 to 4 | 481 | 2184 | 1770 |
| Logs page | 1919 ms | 1719 ms | 6 to 4 | 446 | 2448 | 2028 |
| Search palette (type 11 chars) | 358 ms | 542 ms | 1 | 0 | 0 | 0 |

Where it stands:

- The xterm bundle (about 330 kB) loaded on every page because the dock console tab was imported statically. Dock tabs now load on first open. The fleet chart on the dashboard loads when "Usage details" is expanded.
- `GET /api/v1/deploys/failed`, `/databases` and `/domains` now take `limit` and `offset` and return `X-Total-Count`. Without `limit` the page is 100 rows for failed deploys and 500 for the others. The failed deploys payload with 20k rows dropped from about 90 kB to under 10 kB.
- Apps, nodes, deployments, the audit log and the search palette were already paged or debounced server side, and the long lists are virtualized. The palette row above is within run to run noise, not a regression.
- Update: the dashboard and apps page now use `GET /api/v1/apps-summary` (status, environment and attention counts) and a server paged `GET /api/v1/apps` (`limit`, `offset`, `q`, `status`, repeated `environment`, `tag` with `tag_match=any`, `X-Total-Count`); the nodes page is paged and carries `resource_count` per node; brand logos load one chunk per mark. Numbers below were not re-measured after this change.
- Compression: GET JSON under `/api/` is gzipped when the client accepts it (`APP_API_COMPRESSION=off` disables). SSE, NDJSON and text streams, responses that set a cookie, `Cache-Control: no-store` responses (the secret-bearing ones) and `/api/v1/auth/` are never compressed, which avoids BREACH on secrets.
- Previously not fixed: the dashboard and apps list still fetch the whole apps list (about 314 kB at 1000 apps) because status tiles and filters count client side. A server side summary endpoint is the next step. The thesvg brand logo chunks (about 250 kB each) still load on home, databases and nodes. `npm run check-bundle-size` already fails on the diagram `elk` chunk (1463 kB against a 720 kB budget) before this change.
- Apps seeded here have no running containers, so this measures list rendering and fetching, not live metrics streaming.
