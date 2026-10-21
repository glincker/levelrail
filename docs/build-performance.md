---
title: "Build performance"
description: Cap build time and parallel builds, and read the per-app build duration trend and cache hit rate.
"og:url": "https://docs.levelrail.com/build-performance"
---

Every build is timed, counted against a concurrency limit, and recorded. This page covers the build limits, what is recorded per build, and where to read the trend.

## Timeout, concurrency, memory and CPU limits

| Limit | Environment variable | Default |
| --- | --- | --- |
| Build timeout | `APP_BUILD_TIMEOUT` | `30m` |
| Concurrent builds | `APP_BUILD_MAX_CONCURRENT` | `2` |
| Memory per build | `APP_BUILD_MEMORY_MB` | `0` (no limit) |
| CPU per build, in thousandths of a core | `APP_BUILD_CPU_MILLIS` | `0` (no limit) |

`0` turns a limit off. A build past the concurrency limit waits in a queue, first in first out, and the wait is recorded but does not count against its timeout. A build that runs past the timeout is stopped and fails with the `build_timeout` code (see [Deploy failures](/deploy-failures)).

To change a limit without restarting the control plane, use **Settings, Build performance** in the dashboard (root accounts only) or the CLI. A saved value wins over the environment variable until you reset it.

```bash
levelrail-cli apps build-limits show
levelrail-cli apps build-limits set --timeout 45m --max-concurrent 4
levelrail-cli apps build-limits reset
```

Timeout, memory and CPU can also be set for one app, on the **Deploys** page under **Build limits** or with `--app`. An app value wins over the global one. Concurrency is a control plane wide slot count and cannot be set per app.

```bash
levelrail-cli apps build-limits set --app web --memory-mb 2048 --cpus 1.5
levelrail-cli apps build-limits show --app web
```

Memory and CPU are enforced with a cgroup v2 group per build (`APP_BUILD_CGROUP_ROOT`, default `/sys/fs/cgroup`), so the control plane needs write access to it on a Linux host. Where cgroups are not available the build runs unlimited and the control plane logs a warning. Builds sent to a remote build node are not capped by these two limits. A build killed for exceeding its memory limit is recorded as `memory_limit`, and the Deploys page offers a **Raise memory** button for it, or **Raise timeout** after a timeout.

The limits apply to builds that start after the change. Builds already running keep the timeout they started with.

## What is recorded

Each finished build stores its app, start time, duration, queue wait, outcome (`success`, `failed`, `timeout`, `memory_limit` or `canceled`), and how many of its build steps were served from cache. The newest 200 builds per app are kept. A build canceled while it is still queued never ran and is not recorded.

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

## Making builds faster

The build performance card also shows how much time the cache saved (median cold build against median cached build, over the last 50 builds) and suggestions, each with its fix:

- **Cache turned off** (auto): the app has a cache destination that is disabled. The **Enable cache** button turns it back on.
- **No remote cache configured** (guided): cache hits are low and no destination is set. **Set up cache** opens the app's deploy settings.
- **Few steps reused** (manual): put the lockfile copy and dependency install before copying the source, and add a `.dockerignore`.
- **Builds got slower recently** (manual): the recent median is at least 30% above the earlier one.

```bash
levelrail-cli apps build-speed web          # show the suggestions
levelrail-cli apps build-speed web --fix    # apply the ones the control plane can apply
```

The API is `GET /api/v1/apps/{name}/build-speed`, and agents get the same data from the `get_build_speed` MCP tool.

Remote registry cache hits are reported by BuildKit like local ones, so [build cache](/object-storage) setup shows up here as a rising cache hit rate.
