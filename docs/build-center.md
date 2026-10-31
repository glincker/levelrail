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
