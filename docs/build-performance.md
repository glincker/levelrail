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
| Memory per build | `APP_BUILD_MEMORY_MB` (seeds the `builds.memory_limit_mb` setting) | `0` (auto, see below) |
| CPU per build, in thousandths of a core | `APP_BUILD_CPU_MILLIS` | `0` (no limit) |

`0` turns a limit off, except memory, where `0` means auto. A build past the concurrency limit waits in a queue, first in first out, and the wait is recorded but does not count against its timeout. A build that runs past the timeout is stopped and fails with the `build_timeout` code (see [Deploy failures](/deploy-failures)).

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

### Build memory guard

`builds.memory_limit_mb` is a runtime setting (Settings, Deploys). A saved value wins, `APP_BUILD_MEMORY_MB` only seeds it, and a change applies to the next build. Valid values are `0` (auto) or 256 to 1048576 MiB. A saved per-app or global value from the build limits above still wins over it, and a saved `0` there means unlimited on purpose.

Auto is `min(max(host memory * 60 percent, 512), host memory minus 512)`, never below 256. `APP_BUILD_MEMORY_PERCENT` (default `60`) and `APP_BUILD_MEMORY_RESERVE_MB` (default `512`, room for the control plane) tune it. Examples: 1 GB host, 512 MiB; 2 GB, 1228; 4 GB, 2457; 8 GB, 4915.

The cap is a cgroup memory limit, and the build's swap is limited to the same size so it cannot swap past the cap. Railpack Node builds also get `NODE_OPTIONS=--max-old-space-size` at 75 percent of the cap (`APP_BUILD_NODE_HEAP_PERCENT`) unless the plan already sets `NODE_OPTIONS`; the build log says which. Dockerfile builds keep their own `NODE_OPTIONS`. Builds on a remote build node are not capped here.

A build killed by the out of memory killer (exit 137) fails with a message naming `builds.memory_limit_mb`, the limit, the host memory and the options: raise the limit, use a builds-only node, add swap, or build elsewhere. Stuck diagnostics show the same hint. `doctor` warns (never fails) when the host has under 2048 MiB, the documented floor for a Next.js build (1024 MiB for other builds), and a build on such a host logs the same warning. These floors are documented guidance, not measured.

## Sizing a VPS

Measured on real hosts: the control plane resident memory is 264 MB in a 2048 MiB guest (about 434 MiB used by the whole guest after a clean restart) and 137 to 295 MiB on a 6 GiB Oracle arm64 host. Uptime Kuma idles at about 102 to 130 MiB. n8n idles at about 128 MiB.

Not measured: the idle memory of a Next.js app. The table assumes 150 MiB per small Next.js site, and the site counts are arithmetic, not a benchmark. They leave 20 percent of the RAM free and use 434 MiB for the platform.

| Host | Auto build cap | Small Next.js sites running (assumed 150 MiB each) | Building on the same host |
| --- | --- | --- | --- |
| 1 GB | 512 MiB | not recommended | Use a builds-only node, or build in GitHub Actions or Cloudflare and deploy the image |
| 2 GB | 1228 MiB | about 8 | One build at a time (`APP_BUILD_MAX_CONCURRENT=1`) and a swap file |
| 4 GB | 2457 MiB | about 18 | One build at a time |
| 8 GB | 4915 MiB | about 40 | Two builds at a time |

A build and the running sites share the same RAM, so keep the sites plus the build cap under the host memory. For a 1 GB host, do not build Next.js on it. Next.js per-site memory has not been measured on a real host (pending a host run). See [Frameworks](/frameworks#sizing-a-nextjs-app).

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
