---
description: Measured idle footprint of the control plane at 0, 100, and 500 apps, and how to reproduce it.
---

# Performance

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

<CopyCommand command="scripts/bench-idle.sh" />

The defaults are N = 0, 100, 500 and a 60 second window. For a quicker run:

```bash
BENCH_COUNTS="0 100" BENCH_SECONDS=20 scripts/bench-idle.sh
```

Other knobs: `BENCH_REQUESTS` (requests per endpoint, default 200) and
`BENCH_PORT`. It needs bash, curl, ps, awk, and the Go toolchain, and cleans up
its data directory on exit.
