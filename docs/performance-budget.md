---
title: "Performance budget"
description: The rules every new control plane feature follows so idle cost stays flat as features grow.
---

The control plane is meant to stay small on a 2 vCPU, 8 GB box. Measured idle on such a box: about 200 MB RSS (peak 253 MB), 273 goroutines, 11 threads, 1.5 percent CPU. These rules keep that number from creeping up. Read them before adding a subsystem.

## Rules

1. **Lazy start.** A background loop starts only when its feature is enabled or has work. A disabled experimental feature must not start a goroutine, open a file or allocate a cache.
2. **One loop primitive.** Periodic work goes through `footprint.Every` (jittered first tick, one recorded entry per loop) or registers with `footprint.Track` and calls `Observe` per run. Do not add a bare `time.NewTicker` loop. Every loop appears on the Control plane health page with its interval, last run and cost.
3. **Context and shutdown.** Every loop takes a context and returns when it ends.
4. **Bounded memory.** Every cache has a size or age cap. Large data (search index, exports, archives) lives in SQLite or on disk, never only in RAM. Plugin panel responses and marketplace indexes are capped and expire.
5. **No per-request fan-out.** A request must not start work proportional to the number of apps, nodes or plugins. Aggregate in one query or read a cached snapshot.
6. **Paginate lists.** List endpoints take `limit` and `offset` with a hard cap of 500. Array responses set `X-Total-Count`; struct-shaped responses (orphans, storage orphans, sleep fleet and sleep overview) keep their fields and add a `page` block (`total`, `limit`, `offset`, `next_offset` until the last page) once `limit` or `offset` is sent. Paged routes show `limit` and `offset` in the OpenAPI document, and routes that honor `Idempotency-Key` list that header. UI lists over 50 rows are virtualized.
7. **Bounded streams.** Long lived streams use `startLiveSSE` so they count against `APP_MAX_LIVE_STREAMS`.
8. **MCP toolset budget.** New MCP tools must fit the per-toolset token budget in `internal/mcptools/budget_test.go`. Budgets are lowered, not raised.
9. **Retention.** New stored telemetry has a retention default and is pruned. The metrics database is capped at 1 GiB and pruned hourly.

## Memory limit

At boot the control plane sets a soft Go memory limit of 80 percent of the container (cgroup) or host memory, so the garbage collector works harder before the kernel OOM killer acts. Override the share with `APP_GOMEMLIMIT_PERCENT` (0 disables it), or set `GOMEMLIMIT` yourself, which always wins. Limits under 128 MB are ignored. `GOGC` stays at the Go default (100); set `GOGC` in the environment to trade memory for CPU.

## Shutdown

On SIGTERM the control plane first ends every live SSE stream (clients reconnect on their own), lets a running reconcile pass and any detached queued deploys finish for up to `APP_SHUTDOWN_DRAIN` (default 20s), stops starting new queued deploys, then waits up to 10 seconds for ordinary requests. A pass still running at the limit is cancelled and converges on the next boot, since reconcile is level-triggered. Keep systemd `TimeoutStopSec` above the drain.

## Guard

`go test ./cmd/levelrail -run TestFootprint` boots the real control plane (`run()` on a temp data dir, ephemeral ports, no Docker daemon needed) twice, with every experimental feature off and then on, settles 20 seconds, idles 60 seconds each (`APP_FOOTPRINT_IDLE_SECONDS`) and compares goroutines and RSS with `cmd/levelrail/testdata/footprint_baseline.json`. It fails when goroutines exceed the ratchet by more than 5 or RSS grows more than 15 percent. It is skipped under `-short` and runs in the nightly lane. After an intended change, re-record with `FOOTPRINT_RECORD=1` and say why in the commit. Baselines are per OS (`linux` and `darwin` are recorded); on a new OS the first run logs that it has none until recorded.

Because the harness runs the whole boot path, a new background loop shows up in its loop count and goroutine total. It does not exercise container work (no Docker), so use the health page of a real instance for the cost of a live reconcile.

## Loops and caches in practice

- Sweeps, schedulers and retention loops use `footprint.Every` or `footprint.Track`; the reconcile pass itself is one `reconcile pass` entry. The archive sweeper holds no timer while the archive policy is off and wakes when the policy is saved. The deploy URL syncer does one cleanup pass and exits when `APP_DEPLOY_URL_KEEP` is 0.
- Caps in force: search ranks at most 500 candidates per query; the IAM parse cache holds 2048 documents and 4 MiB and never stores a document over 64 KiB; an account export is refused above 32 MiB and at most 64 export jobs are held in memory; marketplace indexes are limited to 1 MiB; plugin panel responses are limited by `MaxPanelBytes`.
- MCP: `list_apps`, `list_deployments`, `list_deploy_attempts` and `list_audit_log` accept `fields` (comma separated keys kept per item) so an agent can ask for names and statuses only. No toolset may exceed 4000 estimated tokens (`deploys` is 3.9k after canary, import and schedule tools moved to their own toolsets).
