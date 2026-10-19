---
title: "Resource advisor (beta)"
tag: "Beta"
description: "The beta resource advisor suggests memory and CPU limits, flags idle apps, memory growth and disk pressure, and sizes database settings from local telemetry."
---

The resource advisor reads the usage history the platform already keeps on each server and turns it into plain suggestions. It tells you which containers need a memory limit, which limits are too tight or far too generous, which apps sit idle, how to size Postgres for the memory it has, and when a disk will fill. It never changes anything on its own. You apply a suggestion by opening it, reviewing the before and after, and confirming.

It is deterministic: the same history always gives the same findings, and no AI model is involved. The AI assistant and the MCP server can read the findings through the `resource_advice` tool, but the advisor works fine with no AI configured.

## Turn it on

The advisor is off by default. Turn it on in **Settings, Beta features**, or from the CLI:

```bash
levelrail-cli advisor enable
```

`APP_EXPERIMENTAL=resource-advisor` on the control plane turns it on too, and then Settings cannot turn it off. While it is off, the advisor's routes answer `404` with `"code": "experimental_feature_disabled"`, its page, navigation entry, dashboard tile and Optimise findings are hidden, and the attention feed carries no advice. The MCP server reads its own environment, so set `APP_EXPERIMENTAL=resource-advisor` there to expose the `resource_advice` tool.

## Where it shows up

- **Infrastructure, Resource advisor**: every finding, grouped by resource or ordered by estimated saving, with a summary of warnings, applicable changes and memory to reclaim.
- **Resources tab of an app or database**: the **Optimise** panel lists that resource's findings. The evidence is collapsed (usage against the limit, the p95 line and the suggested value), and there is a **Review and apply** button. The cost comparison sits underneath, also collapsed. With the beta off, the panel shows the plain limit suggestion instead.
- **Overview of an app or database**: one line pointing at the Optimise panel when there is advice.
- **Dashboard**: a summary tile linking to the advisor page.
- **Attention feed**: each warning is its own item, and suggestions come as one roll-up marked as recommendations rather than incidents.

## Apply, dismiss and snooze

The advisor can apply only three kinds of action: set a memory limit, set a CPU limit, and turn on [sleep when idle](/sleep-when-idle). Everything else (engine tuning, consolidation, disk cleanup) comes with instructions you follow yourself.

```bash
levelrail-cli advisor findings --resource database:main
levelrail-cli advisor apply memory_limit_missing:database:main        # preview only
levelrail-cli advisor apply memory_limit_missing:database:main --yes  # apply
levelrail-cli advisor snooze memory_limit_missing:database:main --for 14d
levelrail-cli advisor dismiss cpu_limit_missing:app:web
levelrail-cli advisor restore cpu_limit_missing:app:web
```

`POST /api/v1/resource-advisor/findings/{id}/apply` without `{"confirm": true}` returns a preview (`before`, `after`, `confirm_required: true`) and changes nothing. With confirm, the limit is saved to the desired state and, when the container is running, applied to it live; otherwise it takes effect on the next start. If the setting changed after the suggestion was made, apply answers `409` and asks you to review again rather than overwrite your change.

Dismiss hides a finding until you restore it. Snooze hides it for a while (7 days by default, at most 90). Both are stored per finding and everyone on the instance sees the same result.

## Who sees what

Reading findings needs the `read` ability. Findings are filtered to the apps and databases the caller can read, including team scope and IAM visibility. Server-level findings (disk, host memory, consolidation) need `root`. Applying, dismissing or snoozing needs `write` on the route and on the resource itself; turning on sleep when idle needs `deploy` on the app. Switching the beta needs `root`.

## How each rule works

Every rule needs at least `APP_RESOURCE_ADVISOR_MIN_DATA` (24 hours) of history for the resource. With less, the advisor says "Not enough data yet" and suggests nothing. Suggested limits round up to sensible steps: memory to 64, 96, 128, 192, 256, 384, 512, 768 MiB, 1, 1.5, 2 GiB and then 512 MiB steps; CPU to 0.1, 0.25, 0.5, 0.75, 1, 1.5, 2, 3, 4 cores and then whole cores. A CPU suggestion is never below 0.1 core. The same rounding is used by the plain limit suggestion (`resource-recommendation`), so both always agree.

| Finding | When | Suggestion |
| --- | --- | --- |
| Set a memory limit | No memory limit | p95 over the window times the headroom (1.3), never below the observed peak. The benefit is blast radius: a leak OOM-kills this container instead of a neighbour or the control plane. |
| Set a CPU limit | No CPU limit | The larger of p99 and p95 times the CPU headroom (1.5), at least half a core for a database. Informational, since a cap throttles bursts. |
| Limit too tight | p95 at or above 90% of the limit, an OOM-kill line in the logs, or 3 or more restarts with p99 above 80% of the limit | p95 times the headroom, at least 1.25 times (1.5 after an OOM kill) the current limit. |
| Limit far above use | p99 at or below 40% of the limit for the whole window (7 days) | p99 times the headroom, never below the peak. The saving is the difference. |
| Memory keeps growing | A straight-line fit of hourly means with R² of at least 0.8 and growth of at least 20% over the window | An estimate of when it reaches the limit, or the server's memory when there is no limit. Warning under 7 days. For databases it is capped at a suggestion because caches warm up after a restart. |
| No requests recently | An app with a routed domain and no proxied requests for 7 days, running for that long | Sleep when idle after 30 quiet minutes. Apps without a domain (workers) are never assessed. |
| Database looks unused | No app links to it and CPU p99 below 1% for 7 days | Back up, check the restore, then stop it. Only a manual step. |
| Tune engine settings | Postgres, Redis, KeyDB, MySQL or MariaDB with p95 of 192 MiB or more, or any pgvector Postgres | Values sized from the limit, or from the suggested limit when there is none. See below. |
| Small databases on one server | 4 or more databases of one engine on a server, each with p95 at or below 256 MiB | Informational: one instance with a database and role per app, with the tradeoffs listed. Never done automatically. |
| Disk fills soon | A straight-line fit of disk use reaches the disk size within 30 days | Clean up, check retention, or grow the disk. Warning under 7 days. |
| Unused images and build cache | Docker reports at least 1 GiB reclaimable in images and build cache | Use **Clean up Docker**. Docker counts every unused image while cleanup removes dangling images and build cache, so it can free less. |
| Server short of memory | Average use over the last hour at or above 90% of host memory | Cap or move the largest consumers. |
| Swap in use | Average swap over the last hour of 256 MiB or more (Linux) | Cap or move the largest consumers. |
| Top consumers | Three containers hold at least 60% of container memory and well above an even share | Informational: where limits and tuning pay off first. |
| Capacity not reported | A server reports no host memory | Explains why. A single server without a node record (mesh off) does not record host memory per node, which is why the dashboard shows "Capacity not reported"; the advisor reads host memory directly for its own checks. |

### Engine tuning values

For Postgres, with memory M (the limit, or the suggested limit):

- `shared_buffers`: M / 4
- `effective_cache_size`: 3M / 4
- `max_connections`: 50 below 1 GiB, 100 below 4 GiB, 200 above
- `work_mem`: (M minus shared_buffers) / (max_connections times 3), between 1 MB and 64 MB
- `maintenance_work_mem`: M / 16, or M / 4 for pgvector so HNSW and IVFFlat index builds fit in memory

Apply them with `ALTER SYSTEM SET` and `SELECT pg_reload_conf();` (`shared_buffers` and `max_connections` need a restart). For Redis and KeyDB the advisor suggests `maxmemory` at three quarters of M and an eviction policy (`allkeys-lru` for a cache, `noeviction` for queues or primary data). For MySQL and MariaDB it suggests `innodb_buffer_pool_size` at about half of M, in 128 MB chunks.

## Thresholds

Every threshold has an env var on the control plane. A value that does not parse keeps its default and is logged.

| Env var | Default | Meaning |
| --- | --- | --- |
| `APP_RESOURCE_ADVISOR_WINDOW` | `7d` | History the limit and growth rules read |
| `APP_RESOURCE_ADVISOR_MIN_DATA` | `24h` | Minimum history before any advice |
| `APP_RESOURCE_ADVISOR_MEMORY_HEADROOM` | `1.3` | Multiplier over p95 for memory limits |
| `APP_RESOURCE_ADVISOR_CPU_HEADROOM` | `1.5` | Multiplier over p95 for CPU limits |
| `APP_RESOURCE_ADVISOR_TIGHT_RATIO` | `0.9` | p95 over limit that counts as too tight |
| `APP_RESOURCE_ADVISOR_OVERPROVISION_RATIO` | `0.4` | p99 over limit that counts as far above use |
| `APP_RESOURCE_ADVISOR_RESTARTS_TIGHT` | `3` | Restarts in the window that, near the limit, count as too tight |
| `APP_RESOURCE_ADVISOR_GROWTH_MIN` | `0.2` | Minimum growth over the window for a growth finding |
| `APP_RESOURCE_ADVISOR_GROWTH_FIT` | `0.8` | Minimum R² of the growth fit |
| `APP_RESOURCE_ADVISOR_IDLE_WINDOW` | `7d` | Quiet period before an app or database counts as idle |
| `APP_RESOURCE_ADVISOR_IDLE_CPU_PERCENT` | `1` | CPU p99 below which an unlinked database counts as idle |
| `APP_RESOURCE_ADVISOR_SLEEP_MINUTES` | `30` | Idle minutes the sleep suggestion uses |
| `APP_RESOURCE_ADVISOR_TUNING_MIN_MEMORY` | `192Mi` | Database memory at which tuning advice starts |
| `APP_RESOURCE_ADVISOR_SMALL_DB_MEMORY` | `256Mi` | Size below which a database counts as small |
| `APP_RESOURCE_ADVISOR_SMALL_DB_COUNT` | `4` | Small databases of one engine before the consolidation note |
| `APP_RESOURCE_ADVISOR_HOST_PRESSURE` | `0.9` | Host memory use that counts as pressure |
| `APP_RESOURCE_ADVISOR_SWAP_WARN` | `256Mi` | Swap use worth a finding |
| `APP_RESOURCE_ADVISOR_TOP_SHARE` | `0.6` | Share of container memory held by the top three |
| `APP_RESOURCE_ADVISOR_DISK_HORIZON` | `30d` | How far ahead the disk forecast warns |
| `APP_RESOURCE_ADVISOR_RECLAIM_MIN` | `1Gi` | Reclaimable Docker data worth a finding |
| `APP_RESOURCE_ADVISOR_DATABASE_CPU_FLOOR` | `0.5` | Smallest CPU cap, in cores, suggested for a database |
| `APP_RESOURCE_ADVISOR_CACHE_TTL` | `1m` | How long one analysis is reused |

The analysis runs on request and is cached for the TTL. The one Docker call (reclaimable disk) is bounded to 2 seconds and skipped when it times out. The attention feed reads only the cache and refreshes it in the background, so it never waits for an analysis.

## What this does not know

- **Your traffic plan.** It sees the last week, not next week's launch or a monthly batch job. A limit sized from a quiet week can be too small for a busy one.
- **Connections.** It does not measure database connection counts. A database counts as unused only when no app links to it, which the advisor learns from managed references, attachments and plain env values. An app that holds the connection string in a secret makes links unknown, and then no database is flagged as unused.
- **Current engine settings.** The advisor cannot read `shared_buffers` or `maxmemory` from inside the database. Tuning values assume image defaults; check yours with `SHOW` or `CONFIG GET` before applying.
- **Short spikes in older history.** A week of history is read from hourly averages once raw samples age out, so a spike shorter than an hour is smoothed. Limits keep headroom above the highest hourly peak for that reason.
- **Per-database volume size.** Disk forecasting is per server, not per database volume.
- **Remote servers' host memory and swap.** Host memory, swap and disk are recorded for the control plane's own server only; remote nodes report container usage but not host capacity yet.
- **Requests that bypass the proxy.** Idle detection counts requests through the built-in proxy. Traffic straight to a published port or through another proxy is invisible to it.
- **Whether a change is safe for your app.** A CPU cap can slow a startup that bursts; a lower memory limit can kill a job that peaks rarely. Read the evidence and the risk line before applying.

## API

| Method | Path | Ability |
| --- | --- | --- |
| GET | `/api/v1/resource-advisor/findings?resource=app:web&severity=suggest&include=hidden` | read |
| GET | `/api/v1/resource-advisor/summary` | read |
| POST | `/api/v1/resource-advisor/findings/{id}/apply` | write, plus write or deploy on the resource |
| POST | `/api/v1/resource-advisor/findings/{id}/dismiss` | write on the resource |
| POST | `/api/v1/resource-advisor/findings/{id}/snooze` | write on the resource, body `{"duration": "7d"}` |
| POST | `/api/v1/resource-advisor/findings/{id}/restore` | write on the resource |
| GET | `/api/v1/settings/beta-features` | read |
| PUT | `/api/v1/settings/beta-features/{feature}` | root, body `{"enabled": true}` |

Findings carry `kind`, `severity` (`info`, `suggest`, `warn`), `resource`, `title`, `why`, `evidence` (window, sample count, p50, p95, p99, peak, limit and, when filtered to one resource, an hourly series), `impact` (estimated saving, exhaustion time, risk) and `action`. Finding ids are stable (`kind:resource`), so a dismissal survives refreshes.

## Troubleshooting

### Advisor page shows "Not enabled" or no findings appear

Likely cause: The resource advisor beta feature is turned off.

How to confirm: Check Settings > Beta features to see if "Resource advisor" is enabled. Or run `levelrail-cli advisor enable --json | grep enabled`.

Fix: Enable it: `levelrail-cli advisor enable` or in the Settings page, toggle Resource advisor on. The page will refresh and load findings within a minute.

Still stuck: If you see "Not enough data yet", the resource does not have 24 hours of history. Wait a day for the advisor to collect enough data.

### A finding suggests a limit that seems too high or too low

Likely cause: The suggestion is based on the last 7 days of history, which may not represent your actual peak load.

How to confirm: Check the evidence in the finding: the p95 and p99 values show what usage was measured. The peak and hourly series show spikes. If the suggestion looks wrong, verify the resource's actual usage: `levelrail-cli apps metrics <app> --metric memory_bytes --since 7d --step 1h`.

Fix: The suggestion is a starting point. If you know your app will be busier next week, increase the limit by hand. If a finding is consistently off (always too generous, always too tight), adjust the control plane's thresholds (for example, `APP_RESOURCE_ADVISOR_MEMORY_HEADROOM`).

Still stuck: Read the "What this does not know" section above: the advisor uses historical data, not future predictions.

### Memory limit increase did not help, app still crashing

Likely cause: The app has a memory leak, or the growth is faster than the suggestion accounted for.

How to confirm: Check the container restarts: `levelrail-cli apps status <app> | grep restarts`. Check recent metrics: `levelrail-cli apps metrics <app> --metric memory_bytes --since 24h`. If memory climbs steadily even after a restart, there is likely a leak.

Fix: Investigate the app for memory leaks (check logs with `levelrail-cli logs signatures <app> --level warn --since 1h`). Increase the limit further as a temporary fix while debugging. Once fixed, the advisor's next run will suggest a smaller limit.

Still stuck: See [Investigating a spike](/observability#investigating-a-spike) for debugging approaches.

