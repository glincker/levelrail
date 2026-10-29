---
title: "Whole-server recovery"
description: After restoring the control plane, pull every app volume and database back from its latest backup, with a dry-run plan and a per-resource report.
"og:url": "https://docs.levelrail.com/whole-server-recovery"
---

[Disaster recovery](/disaster-recovery) rebuilds the control plane. Whole-server recovery is the second half: it reads the restored control plane's apps and databases, finds the latest usable backup of each, and restores them.

## How it works

1. **Plan** (dry run, the default). For every database and every named app volume it picks the newest successful backup. A resource with no successful backup, or whose newest backup is older than `APP_REHYDRATE_MAX_BACKUP_AGE`, is blocked. Engines that keep nothing on disk are skipped.
2. **Run** restores the planned resources through the same restore runners as the per-resource restore routes, at most `APP_REHYDRATE_CONCURRENCY` at a time. Each resource gets its own status, so one failure does not stop the others.
3. **Report** shows the outcome per resource and the data loss window, which is the age of the oldest backup used. The last report is saved and survives a control plane restart.

A run overwrites live databases and volumes. It needs a root-scoped token and is audited like any other root action.

## Settings

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_REHYDRATE_MAX_BACKUP_AGE` | `168h` | Oldest backup a run will use. `0` disables the check. |
| `APP_REHYDRATE_CONCURRENCY` | `2` | Restores running at once. |

## CLI

```bash
levelrail-cli control-plane rehydrate plan            # dry run
levelrail-cli control-plane rehydrate run --yes       # restore and wait for the report
levelrail-cli control-plane rehydrate status --json   # live or last report
```

`plan` exits 1 when any resource is blocked. `run` exits 1 when the result is partial or failed. Add `--allow-stale` to use backups older than the age limit, and `--no-wait` on `run` to return at once.

## Dashboard

Settings, Recovery report: run a plan, confirm, and watch each resource's status.

## API

| Route | Purpose |
| --- | --- |
| `POST /api/v1/control-plane/rehydrate/plan` | Dry run, changes nothing. |
| `POST /api/v1/control-plane/rehydrate/run` | Starts the restores, returns 202. 409 if a run is active. |
| `GET /api/v1/control-plane/rehydrate/report` | Live or last report, 404 before the first plan or run. |

## Limits

Restores need the master key to be set, because backup credentials are stored encrypted. Databases and volumes are restored from the control plane's own backup history, so a resource that never had a backup stays blocked.
