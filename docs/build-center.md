---
title: "Build center"
description: "One place to see and control builds: the queue, running builds, history with duration and cache hit ratio, and the BuildKit cache of each node."
"og:url": "https://docs.levelrail.com/build-center"
---

One place to see and control builds: the queue, running builds, history with
duration, cache hit ratio, image size change and the node that ran each build,
plus the BuildKit cache of each node.

Open it from the dashboard under Builds, or use the CLI and API below.

## What you see

| Column | Meaning |
| --- | --- |
| Status | queued (with its place in line), running, succeeded, failed, canceled |
| Time | build duration, measured by the builder when it ran, else the deploy time |
| Cache | cached build steps over total steps |
| Image | image size, and the change against the app's previous successful build |
| Node and platform | where the build ran and the OS/arch it targeted |

A build that runs for over an hour, or a queue that waits more than five
minutes with nothing running, is flagged "may be stuck" and its Cancel button
becomes the primary action.

## Concurrency

`APP_BUILD_MAX_CONCURRENT` (default 2, 0 for unlimited) is a limit **per node**:
two nodes with a limit of 2 run up to four builds at once. Builds past the limit
wait in the queue in arrival order. The effective value can also be changed in
Settings under build performance. `APP_BUILD_TIMEOUT` still stops a single build.

## Cancel and retry

- Cancel drops a queued build or stops a running one before it can change the
  serving release. A build that already cut traffic cannot be canceled, roll
  back instead.
- Retry rebuilds the latest commit of the connected branch for a failed or
  canceled build. It needs a connected git source and is not offered for plain
  image or compose deploys.

## Failure hints

A failed build is matched against the same deterministic rules the deploy
diagnosis uses (out of disk, wrong CPU architecture, missing env and so on).
The hint shows the cause and a suggestion. When the cause is a full disk, the
row and the top of the page get a "Free build cache" button that opens the
prune preview.

## Build cache

Per node, the page and CLI list BuildKit cache records with size, last use and
whether a running build holds them. Pruning always previews first: it lists the
idle records older than the chosen age (default 168 hours) and the bytes it would
free. Records an in-flight build holds are never removed. Pruning needs a root
token. Only the control plane's own node reports its cache today, other nodes
show "not available" until the agent exposes it.

## Build network

What a build's `RUN` steps can reach, from the code in `internal/build`:

- Builds run on the BuildKit that is embedded in the node's Docker Engine,
  reached through the engine's own API. Levelrail does not set a network for
  `RUN` steps and does not grant the `network.host` entitlement, so a
  `RUN --network=host` step is refused by BuildKit.
- With the default policy (`open`) Levelrail applies no restriction. A `RUN`
  step gets the engine's default build network, so it can reach whatever that
  network can route to, which may include the cloud metadata address, the
  Docker gateway and private LAN ranges. This has not been probed on a real
  host yet; treat those ranges as reachable until a probe says otherwise.
- Base image pulls and `ADD <url>` are performed by BuildKit outside `RUN`
  steps and are not affected by any setting below.
- The managed buildkitd supervisor (`internal/build/buildkitd`) starts its
  container with no network, but it is not used by the build path yet.

The policy is the runtime setting `security.build_network`. Change it in the
dashboard settings card, with `levelrail settings runtime set
security.build_network none`, or through the settings API. A saved value wins,
is read on every build and needs no restart; `settings runtime get` shows the
effective value and whether it is saved, from the environment or the default.
`APP_BUILD_NETWORK` only seeds the default when nothing is saved. Agents have
no settings store and read the variable on every build.

| Value | Effect |
| --- | --- |
| `open` (default, also when unset) | No restriction, as described above. |
| `none` | Every `RUN` step runs without a network interface. Dockerfile builds set `force-network-mode=none`; Railpack builds get no network on every exec step. Steps that download packages (`npm install`, `pip install`) fail, so it suits builds with vendored dependencies. |
| `block-private` | Reserved for blocking metadata and private ranges while keeping the internet. No build path can enforce it yet, so builds fail with an explanation instead of running unprotected. |

A value outside this list is refused when saved. The default stays `open`;
whether to change it is an owner decision made after the host probe.

## Platforms

Builds target the platform of the node that runs them, shown on every row.
Multi-platform (buildx style) builds are not supported yet, so there is no
platform selector.

## CLI

```
levelrail-cli builds list [--app A] [--status S] [--limit N] [--json]
levelrail-cli builds show <id> [--json]
levelrail-cli builds cancel <id>
levelrail-cli builds retry <id>
levelrail-cli builds cache [--node N] [--json]
levelrail-cli builds prune [--node N] [--older-than-hours H] [--apply]
```

`prune` previews unless `--apply` is given. `<id>` is the deploy id shown by
`builds list`; follow a live build log with `apps deploys logs <app> <id>`.

## API

| Route | Ability |
| --- | --- |
| `GET /api/v1/builds` | read |
| `GET /api/v1/builds/{id}` | read |
| `POST /api/v1/apps/{name}/builds/{id}/cancel` | deploy |
| `POST /api/v1/apps/{name}/builds/{id}/retry` | deploy |
| `GET /api/v1/builds/cache?node=` | read |
| `POST /api/v1/builds/cache/prune` (`apply` false previews) | write sensitive |

## MCP

`list_builds`, `get_buildkit_cache` and `preview_build_cache_prune` are read
tools. `apply_build_cache_prune` is only available in full mode.
