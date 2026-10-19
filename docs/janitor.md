---
title: "Automatic cleanup"
tag: "Beta"
description: Reclaim disk on the control plane node on a schedule, with preview and quarantine before anything is deleted.
"og:url": "https://docs.levelrail.com/janitor"
---

Builds and deploys leave data behind. The janitor removes what nothing uses, in small bounded passes, and records what it did. Open it from Settings, Advanced, Automatic cleanup.

It starts in preview mode: scheduled passes list what they would do and remove nothing. Turn off "Preview only" in the policy to let it act.

## What it cleans

| Stage | Removes | Never touches |
| --- | --- | --- |
| Stopped containers | Stopped containers no app or database still wants | Any container the reconciler may restart |
| Dangling images | Untagged images only | Tagged images, which include every rollback target |
| Build cache | Idle cache past the age limit, and all idle cache when the cache is over its budget or disk is short | Cache an in-flight build holds |
| Orphaned volumes | Named volumes no app uses, after a quarantine | Database volumes (`db-*`), mounted volumes, volumes a live service or upgrade snapshot references |
| Old deploy attempts | Failed, superseded, canceled and held attempts past retention | Succeeded attempts, pinned rollback targets, each app's newest attempts |

Containers and images are cleaned only when free space falls below the disk budget (default 15%). Build cache, volumes and attempts are time driven and run every pass.

Pipeline artifacts already expire on their own retention (see [pipelines](/pipelines)), so they are not repeated here.

## Preview and quarantine

- Preview (`janitor run --dry-run`, or the Preview button) lists every candidate and changes nothing.
- An orphaned volume is first quarantined: the pass records when it was first seen. It is deleted only when the quarantine (default 14 days) has ended, "Delete volumes after quarantine" is on, and it is still unused. At most 5 volumes are deleted per pass. A volume that becomes referenced again leaves quarantine.

## Run now and the disk alert

Run now does a forced pass: containers and images are cleaned even above the budget. When free space is below budget the page shows a warning with a "Clean up now" button, and the disk space fix in the Fixes list runs the same pass. Every pass that changes something is stored with the bytes it reclaimed.

## CLI

```
levelrail-cli janitor status
levelrail-cli janitor run --dry-run
levelrail-cli janitor run --if-needed
levelrail-cli janitor policy --dry-run=false --delete-orphan-volumes=true
```

The MCP tool `get_janitor` returns the state and, with `preview`, a dry run.

## API

`GET /api/v1/system/janitor`, `GET /api/v1/system/janitor/runs`, `PUT /api/v1/system/janitor/policy` and `POST /api/v1/system/janitor/run?dry_run=true&force=true`. Changing the policy and running need a root token.

## Limits

- It cleans the control plane's own node. Other nodes keep using the per-node build center and image tools.
- Disk free is measured on the control plane data directory.
