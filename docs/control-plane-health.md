---
title: "Control plane health"
description: What the control plane measures about itself, which limits it enforces, and the fixes it can apply.
"og:url": "https://docs.levelrail.com/control-plane-health"
---

The control plane watches its own saturation so a busy instance tells you before it slows down. Open **Settings, Control plane health**, or run `levelrail-cli system health`.

## What is measured

- Process: goroutines, heap in use, last GC pause.
- Database: query latency p50, p95 and p99, writes that waited for the single SQLite connection (the write queue), reads that waited for a free read connection (the read queue, with the pool size and how many connections are busy; shown as "read pool off" when `APP_DB_READ_CONNS` is 0 or the database is in memory, because reads then use the writer), write ahead log size, and a slow query log.
- Reconcile loop: last pass duration, p50 and p95, the slowest controller, and seconds since the last pass finished (event lag).
- Live streams: open log, deploy and event streams against the cap.
- Footprint: resident memory, OS threads, CPU, the metrics database size, the soft memory limit, and every background loop with its interval, last run and share of a core. See [Performance budget](/performance-budget).

Latency covers statements issued through the store, over the last 512 queries. Slow statements keep the SQL text only, never arguments.

## Findings and fixes

Each measurement above its threshold becomes a warning, or critical at twice the threshold. Where the control plane can fix it, the page shows a button.

| Finding | Fix button | CLI |
| --- | --- | --- |
| Write ahead log is large | Checkpoint now (truncating checkpoint) | `system health --fix wal_checkpoint` |
| Slow database queries | Refresh query stats (`PRAGMA optimize`) | `system health --fix db_optimize` |
| Many slow queries | Clear slow log | `system health --fix reset_slow_log` |
| Metrics database is large | Prune and compact metrics (retention, size cap, then VACUUM when over a quarter of the file is free) | `system health --fix telemetry_prune` |
| The control plane database is damaged | Restore the newest verified snapshot (applied at the next start, see [resilience](/resilience#database-integrity-and-guided-restore)) | `system health --fix integrity_restore` |
| A controller keeps failing and was paused | Reset protection and resume | `system health --fix reset_reconcile_guard` |
| A burst of events hit the rate limit | Reset protection and resume | `system health --fix reset_reconcile_guard` |

Goroutines, heap, reconcile pass, event lag and stream findings have no automatic fix. The page says what to look at, and a restart (`system restart`) is the last resort.

Applying a fix needs a root token. Reading health needs read access. The API is `GET /api/v1/system/control-plane-health` and `POST /api/v1/system/control-plane-health/fix` with `{"fix": "<id>"}`.

## Guards that are always on

- Live streams (deploy logs, resource logs, pipeline logs, the deployments stream) are capped. Past the cap a new stream gets `503` with `Retry-After`. Set `APP_MAX_LIVE_STREAMS` (default 500).
- The write ahead log is checkpointed every minute and capped at 64 MiB after a checkpoint, so a long reader cannot let it grow without bound.
- The app list is paginated (`limit`, `offset`) with a hard cap, and API requests are rate limited per actor.

## Thresholds

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_CPHEALTH_GOROUTINES` | 5000 | goroutine count |
| `APP_CPHEALTH_HEAP_MB` | 1536 | heap in use |
| `APP_CPHEALTH_DB_P95_MS` | 100 | database p95 |
| `APP_CPHEALTH_WAL_MB` | 128 | write ahead log size |
| `APP_CPHEALTH_SLOW_QUERIES` | 25 | slow queries since start |
| `APP_CPHEALTH_RECONCILE_PASS_S` | 20 | last reconcile pass |
| `APP_CPHEALTH_EVENT_LAG_MULTIPLE` | 3 | resync intervals without a finished pass |
| `APP_CPHEALTH_STREAM_PERCENT` | 80 | share of the stream cap in use |
| `APP_CPHEALTH_RSS_MB` | 1024 | resident memory |
| `APP_CPHEALTH_THREADS` | 200 | OS threads |
| `APP_CPHEALTH_TELEMETRY_MB` | 768 | metrics database file |
| `APP_CPHEALTH_STORM_EVENTS` | 100 | passes or resource runs held back by rate limits in the last 5 minutes |
| `APP_GOMEMLIMIT_PERCENT` | 80 | soft Go memory limit as a share of container or host memory, 0 disables, ignored when `GOMEMLIMIT` is set |
| `APP_DB_SLOW_QUERY_MS` | 200 | a statement slower than this is logged as slow |
| `APP_DB_READ_CONNS` | 4 | read-only SQLite connections for dashboard reads, so they do not queue behind reconcile writes. 0 sends reads to the single writer connection |

See [Benchmarks](/benchmarks#control-plane-at-1000-apps) for measured latency at 1000 apps.
