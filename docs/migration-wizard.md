---
title: "Migration wizard"
sidebarTitle: "Migration wizard"
description: Move apps, databases and volumes off Coolify, Dokploy, CapRover or an old server in one guided, resumable job, with live progress, DNS checks and a safe-to-delete gate.
"og:url": "https://docs.levelrail.com/migration-wizard"
---

The wizard walks one migration from "connect the old platform" to "safe to delete it". It is the dashboard face of an import job: the same job is reachable from the API, the CLI (`--resume`) and a read-only MCP tool, and it survives a restart of the control plane.

Open it in the dashboard under **Settings, Import**. Read the [migration overview](/migrate-to-levelrail) first for the flow and the downtime table.

## How far to trust this

No real Coolify, Dokploy or CapRover instance was available while this was built. The wizard was exercised against fixture servers shaped from each platform's documented API, with fake credentials, plus unit and API tests. That proves the job logic, the read-only guarantee (the fixture servers reject every non-GET), resumability and the safe-to-delete rules. It does not prove that a field name or route matches your version of the old platform. Run **Test connection** and a plan against your own instance first, and compare two or three apps you know well before you rely on a result.

## The steps

| Step | What happens | Changes the old instance |
| --- | --- | --- |
| Connect | Pick Coolify, Dokploy or CapRover (or a Docker snapshot, a compose file, a Dokku env dump, or an SSH login), enter the URL and token, and press **Test connection**. | No |
| Inventory | Every app, database, volume, domain, cron job and environment variable count the importer found, with a support status per resource. Select what to move, set hostname mappings and the name collision rule. | No |
| Preflight | The dry-run plan: what becomes what, what needs you, and what blocks staging. For Coolify, Dokploy, CapRover and SSH sources the exit report lists every resource as imported, manual or skipped with the reason. | No |
| Stage | Creates each selected app here, stopped and unrouted. Progress is live. A failed app has its own **Retry** and never blocks the others. | No |
| Move images, Build and verify | Moves host-built images, then builds or pulls each app and waits for its health check. | No |
| Volumes | Copies volume data over SSH where the source allows it, with file and byte counts, or prints the copy command. Databases use `migrate db-copy` and are compared table by table. | No (read only) |
| Cutover | Per domain: lower the TTL, enable routing, change DNS. **Check DNS now** and **Check the app responds here** probe each domain. | No |
| Safe to delete | The final checklist. It turns green only when every gate passes. | No |

Nothing in any step stops, edits or deletes anything on the old instance. To go back at any point before the first write lands here, point the DNS record at the old server again.

## Credentials

- The source credential is sent in a request body, kept in server memory for the session (for a limited time set by `APP_MIGRATE_HUB_PASSWORD_TTL`) and wiped when the session is forgotten or expires. It is never written to the database, the logs or the audit log, and no response echoes it.
- After a restart of the control plane the credential is gone but the job is not. The wizard asks for the token again and continues.
- Jobs whose source is an SSH login or a pasted file cannot be reconnected from the CLI. Reconnect them in the dashboard.

## Resuming a job

A job is an import session. Everything it did is stored, so you can stop at any point, restart the control plane, and carry on. What is re-run is safe to re-run: staging skips apps that already exist, and a build interrupted by a crash shows as retryable instead of staying "building".

```
levelrail-cli migrate coolify --resume appimp-123
levelrail-cli migrate --resume appimp-123 --retry --token-stdin
levelrail-cli migrate --resume appimp-123 --follow
levelrail-cli migrate --resume appimp-123 --retire --probe --ack source-backup,traffic-cycle
```

`--retry` stages again the items that failed (or verifies again the ones a restart interrupted) and reconnects the source first when it has to. `--retire` exits `1` until the old instance is safe to delete, so a script can wait on it. `levelrail-cli import apps --from dokploy --url URL --token-stdin --test` runs the connection test alone.

## The safe-to-delete checklist

| Gate | Passes when |
| --- | --- |
| Apps built and healthy | Every selected app is verified or routed here |
| Resources the importer could not move | None were left behind, or you confirmed you moved them by hand |
| Volumes copied | Every volume of a selected app is marked copied |
| Database copies verified | Every database the apps use has a verified copy with matching row counts |
| Apps serving their domains | Every selected app with a domain has routing enabled |
| DNS points here | A live lookup of every domain returns this server |
| Apps answer from this server | A live request through this server's own address, with the domain as the name, gets a trusted certificate and a 200 from the health path |
| Old instance backed up | You confirmed it (a person has to) |
| Full traffic cycle | You confirmed it (a person has to) |

A gate that has no evidence yet is **not checked**, never green. The live gates only run when you press **Run the live checks** (or pass `--probe`), because they touch the network. Reading the checklist from the API or the MCP tool never does.

If something goes wrong the checklist also prints the rollback guidance: before the first write lands here a rollback is a DNS change. After a database takes writes here, copy the data back to the old database first, or those writes are lost. Stop the old instance and keep it stopped for a few days before you delete it.

## API

All routes are under `/api/v1/migration/apps`. Every route that carries or uses the source credential needs the write-sensitive ability.

| Route | Purpose |
| --- | --- |
| `POST /test` | Read the source once and report what it holds. Stores nothing |
| `POST /sessions`, `POST /sessions/{id}/connect` | Create a job, or re-attach the credential to one |
| `GET /sessions/{id}/progress` | Compact, secret-free job state with the next actions |
| `GET /sessions/{id}/events` | The same state as server-sent events, resumable with `Last-Event-ID` |
| `POST /sessions/{id}/stage` | Stage the selected apps, or only the ones in `items` (retry) |
| `POST /sessions/{id}/verify` | Build and health-check, optionally for named `items` only |
| `POST /sessions/{id}/probe` | `dns`, `http` or both checks for one domain of the job |
| `GET`, `POST /sessions/{id}/retire` | The safe-to-delete checklist; `POST` can run the live probes and take confirmations |
| `POST /api/v1/migration/exit-report` | The exit report for any live source |

The event stream polls stored state at the interval set by `APP_MIGRATE_EVENTS_INTERVAL` (default one second) and sends a keep-alive every `APP_MIGRATE_EVENTS_HEARTBEAT`. Probe timeouts come from `APP_MIGRATE_PROBE_TIMEOUT`.

## AI agents

`get_migration_job_status` is a read-only MCP tool. It reports the phase, progress, failed items with their reasons, the next steps and which safe-to-delete gates are open. It never runs a probe and cannot change a job. Text written by the source (app names, error reasons) is untrusted input.

## What it does not do yet

- Environments of one Coolify project share one project here.
- Database data is copied by a command you run or by the data copy API, not by the staging button, and a database that takes writes during the final copy needs a write pause you arrange.
- Volume copies marked by hand are taken at your word. Only the SSH volume copy records file and byte counts.
- Network-level facts of the old platform (custom Docker networks, team members, notification channels) are listed as invisible in the [gap report](/coolify-exit-gap-report), not migrated.
- Nothing here has run against a real production instance of the old platforms.
