---
title: "Build performance"
description: Cap build time and parallel builds, and read the per-app build duration trend and cache hit rate.
---

Every build is timed, counted against a concurrency limit, and recorded. This page covers the two limits, what is recorded per build, and where to read the trend.

## Timeout and concurrency limit

| Limit | Environment variable | Default |
| --- | --- | --- |
| Build timeout | `APP_BUILD_TIMEOUT` | `30m` |
| Concurrent builds | `APP_BUILD_MAX_CONCURRENT` | `2` |

`0` turns a limit off. A build past the concurrency limit waits in a queue, first in first out, and the wait is recorded but does not count against its timeout. A build that runs past the timeout is stopped and fails with the `build_timeout` code (see [Deploy failures](/deploy-failures)).

To change a limit without restarting the control plane, use **Settings, Build performance** in the dashboard (root accounts only) or the CLI. A saved value wins over the environment variable until you reset it.

```bash
levelrail-cli apps build-limits show
levelrail-cli apps build-limits set --timeout 45m --max-concurrent 4
levelrail-cli apps build-limits reset
```

The limits apply to builds that start after the change. Builds already running keep the timeout they started with.

## What is recorded

Each finished build stores its app, start time, duration, queue wait, outcome (`success`, `failed`, `timeout` or `canceled`), and how many of its build steps were served from cache. The newest 200 builds per app are kept. A build canceled while it is still queued never ran and is not recorded.

Builds are keyed by the image repository name, which is the app name for git and manual deploys.

## Reading the trend

The app's **Deploys** page shows a build performance card for the last 20 builds: median (p50), p95, cache hit rate and the number of failed or timed out builds, with a bar per build.

- Percentiles use successful builds only. Failed, timed out and canceled builds are counted, not timed.
- Cache hit rate is cached steps over all steps across successful builds. It shows `n/a` when no build reported steps.
- A successful build with a negative duration (clock skew) is counted but left out of the timings.

From the CLI:

```bash
levelrail-cli apps build-stats web --limit 50
```

Over the API, `GET /api/v1/apps/{name}/build-stats?limit=50` returns the same trend plus every run, and `GET /api/v1/build-limits` returns the limits in effect. See the [API reference](/api-reference).

Remote registry cache hits are reported by BuildKit like local ones, so [build cache](/object-storage) setup shows up here as a rising cache hit rate.
