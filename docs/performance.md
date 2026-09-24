# Performance

Measured idle footprint of the control plane at 0, 100, and 500 apps, and how
to reproduce it.

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
Before is `origin/main` at the start of this work, after includes the two
fixes below.

| Apps | RSS MB (before / after) | CPU % (before / after) |
| --- | --- | --- |
| 0 | 58.2 / 67.7 | 0.03 / 0.03 |
| 100 | 88.5 / 82.6 | 0.42 / 0.30 |
| 500 | 95.6 / 90.8 | 2.05 / 1.60 |

Latency p50 / p95 in ms, after the fixes (before in brackets where measured):

| Endpoint | 0 apps | 100 apps | 500 apps |
| --- | --- | --- | --- |
| GET /apps | 0.14 / 0.21 | 0.94 / 1.41 (0.92 / 1.47) | 3.69 / 4.59 (3.99 / 6.85) |
| GET /apps/{name} | n/a | 0.18 / 0.43 | 0.15 / 0.20 (0.16 / 0.20) |
| GET /system/status | 0.70 / 1.32 (364 to 398 p50) | 0.66 / 0.85 | 0.53 / 0.83 |
| GET /system/doctor | 231.52 / 459.98 (210 to 267 p50) | 222.72 / 403.57 | 217.19 / 450.71 |
| GET /certificates | 0.11 / 0.21 | 0.14 / 0.24 | 0.14 / 0.23 |
| GET /deploys/failed?since=24h | 0.12 / 0.18 | 0.12 / 0.20 | 0.32 / 0.36 |

## What changed

- `GET /system/status` called Docker disk usage on every request, costing
  roughly 370 ms. The result is now cached for 30 seconds; errors are never
  cached. p50 drops from about 380 ms to under 1 ms.
- A suspended app's reconcile listed containers twice (container teardown,
  then egress sidecar teardown). It now lists once and shares the result.
  Suspended-app CPU at 500 apps fell from 2.05% to 1.60%.

`GET /system/doctor` is still about 220 ms because it runs live checks
(Docker, DB, and others) by design. It is not on any polling path.

## Caveats

- One machine, one run per configuration. Expect run-to-run variance of
  roughly 10 to 15% on RSS and CPU; the small RSS and CPU deltas at 0 and 100
  apps are within noise. The status endpoint change is not.
- No containers are running. Real apps add Docker event, health probe, and
  metrics collection load that this benchmark does not measure.
- Dev build in dev mode, not a release binary. Loopback client, so no network
  latency is included.
- Apps are suspended, which exercises the reconcile resync path but not
  running-app reconciliation.

## Findings while building the benchmark

- BSD `seq` (macOS) misbehaves with some argument shapes, so the script
  builds request lists with plain loops over `seq 1 N` only.
- The default API rate limits throttle a 200-request burst. The script raises
  `APP_API_RATE_LIMIT_READ_RPM` and `APP_API_RATE_LIMIT_WRITE_RPM`.
- The brand file is resolved relative to the working directory. Run the script
  from the repo root (it does), or set `APP_BRAND_FILE` if running the binary
  from elsewhere.

## Re-running

```sh
scripts/bench-idle.sh                                # N = 0 100 500, 60s window
BENCH_COUNTS="0 100" BENCH_SECONDS=20 scripts/bench-idle.sh
```

Other knobs: `BENCH_REQUESTS` (requests per endpoint, default 200) and
`BENCH_PORT`. It needs bash, curl, ps, awk, and the Go toolchain, and cleans up
its data directory on exit.
