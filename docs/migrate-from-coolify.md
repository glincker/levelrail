---
title: "Migrate from Coolify"
sidebarTitle: "From Coolify"
description: Move Coolify apps, databases, env vars and domains to Levelrail with a read-only plan, staged copies and a reversible cutover. Coolify keeps serving.
"og:url": "https://docs.levelrail.com/migrate-from-coolify"
---

Coolify has the most complete path. You can stage apps, move images that were built on the Coolify host, copy databases with row count checks, and switch each domain with a rollback. Read the [migration overview](/migrate-to-levelrail) first for the flow and the downtime table.

## Prerequisites

- Levelrail installed on the new server, signed in with the CLI (`levelrail-cli auth login`).
- A Coolify API token. Use the `read:sensitive` ability, or secret values come back empty.
- The Coolify URL reachable from the Levelrail control plane. For a private address, set `APP_IMPORT_ALLOW_PRIVATE_NETWORKS=true` on the control plane and pass `--allow-private`.
- For apps built on the Coolify host: SSH access to it with a login that can run `docker` (root or the `docker` group).
- For databases: a database user on each Coolify database, and a route from the Levelrail node to its port.

## What imports automatically

| Item | Result |
| --- | --- |
| Applications | Created stopped and not routed. Image or git source, build method (Nixpacks becomes Railpack), first exposed port, replicas, health check and memory and CPU limits |
| Environment variables | Imported, secret-looking names through the encrypted secrets store. Preview duplicates are dropped |
| Domains | Read and held back until you enable routing |
| Databases | PostgreSQL, MySQL, MariaDB, MongoDB and Redis, created empty with the major version from the image tag |
| Scheduled tasks | Enabled tasks become scheduled jobs that run the command inside the app. Staged apps get them disabled until cutover |
| Persistent storages | Created as empty volumes on each app |

## What needs a manual step

- **Database contents** are copied by a separate step ([below](#databases)).
- **Volume contents** are copied by `rsync` commands you run.
- **Images built on the Coolify host** exist nowhere else. Use the Move images step.
- **Git-built apps** do not build by themselves. Connect the repository (with credentials for a private one) and run a build, or let verify do it.
- **Old database hostnames** in env vars are only rewritten through a mapping you confirm.
- **Secrets the token could not read** are skipped and flagged.
- **Extra ports, raw host port mappings, command-style health checks, custom build and start commands, basic auth, custom labels, webhooks and backup schedules** are called out as needs attention. Nothing is dropped without a row in the report. See the [gap report](/coolify-exit-gap-report).

## What cannot be migrated

- Coolify one-click services and Docker Compose apps (multi-service). They are listed as skipped. Deploy their compose file with [compose apps](/compose-apps).
- TLS certificates, DNS records, deploy history, logs and metrics. Certificates are issued here on first use.
- ClickHouse, KeyDB and Dragonfly databases. They are listed as skipped, and the copy step does not handle them. Use their own dump tools and the [restore tools](/backups-and-storage).

## Exit report and cutover checklist {#exit-report-and-cutover-checklist}

To leave Coolify completely, start with a dry run. It reads Coolify (GET requests only), creates nothing, and prints one row per resource with its outcome: **imported**, **manual** (a person finishes it) or **skipped** (it will not move, with the reason). It also prints the DNS switch order and an ordered cutover checklist.

```bash
levelrail-cli migrate coolify --url https://coolify.example.com --token "$COOLIFY_TOKEN" --dry-run
levelrail-cli migrate coolify --url https://coolify.example.com --token "$COOLIFY_TOKEN" --dry-run --json --only web,api
```

After staging, ask the control plane for the same report. Apps that already exist here show as imported, so it doubles as the post-migration verification report:

```bash
levelrail-cli migrate coolify --url https://coolify.example.com --token "$COOLIFY_TOKEN" --verify
levelrail-cli import apps --session appimp-123 --exit-report
```

The dashboard shows the report on the import page ("Exit report"), the API serves it at `POST /api/v1/migration/coolify/exit-report` and `GET /api/v1/migration/apps/sessions/{id}/exit-report`, and the read-only MCP tool is `get_app_import_exit_report`.

How to read it:

- **Skipped** rows never move on their own. Compose apps, one-click services and engines without a managed equivalent land here.
- **Manual** rows are the work list: volume data, database data (with the exact `migrate db-copy` command), backup schedules, webhooks on your code host, a proxy that owns ports 80 and 443.
- The report says `ready_to_retire` only when nothing is manual or skipped.

DNS switch order, least risky first:

1. Stateless apps (no volume, no database). A bad switch loses nothing.
2. Apps with a volume or a database. Pause writes on the old app, run the last data copy, then switch.
3. Apps with both. Switch last, one at a time.

Keep Coolify as the fallback:

- Do not stop or delete anything on Coolify until the app has run a full traffic cycle here.
- Lower the TTL ahead of time. A rollback restores the previous record (`levelrail-cli import cutover rollback`) and the old app answers again.
- For an app with a database, the fallback holds only until the first write lands here. After that, copy the data back before rolling back.
- Before you delete Coolify, keep a copy of its `.env` (the `APP_KEY` decrypts everything it stored) and a dump of its own database.

The audit behind this report, including what is not proven because no real Coolify instance was available, is in the [gap report](/coolify-exit-gap-report).

## Steps

<Steps>
<Step title="Plan">

Dashboard: Settings, Import from another platform, "Move apps from Coolify". CLI:

```bash
export APP_IMPORT_SOURCE_TOKEN=...
levelrail-cli import apps --from coolify --url https://coolify.example.com --plan
```

Every app and database gets a verdict (ready, ready with notes, needs attention, unsupported) with the reason. Nothing is created.

</Step>
<Step title="Stage">

```bash
levelrail-cli import apps --from coolify --url https://coolify.example.com --apply --only web,api
```

The apps are created stopped and unrouted. The output includes a session id such as `appimp-123`. Re-running skips what exists. Use `--map OLD_DB_HOST=NEW_DB_HOST` for hostnames you want rewritten, shown with a masked before and after.

</Step>
<Step title="Move images built on the Coolify host">

```bash
levelrail-cli import apps --session appimp-123 --transfer-images --ssh root@old.example.com --ssh-key ~/.ssh/id_ed25519
levelrail-cli import apps --session appimp-123 --images
```

It runs `docker save` over SSH and checks the loaded image by content. The target must be the control plane node for now.

</Step>
<Step title="Copy databases" id="databases">

```bash
export APP_MIGRATE_SOURCE_DB_PASSWORD=...
levelrail-cli migrate db-copy main --host db.old.example.com --user app --database app
levelrail-cli migrate db-status
```

For a whole server with many databases, use the Migration hub (same Settings page) or `levelrail-cli migrate server --engine postgres --host db.old.example.com --user admin --password-stdin --plan`, then `--apply`.

</Step>
<Step title="Copy volumes">

```bash
levelrail-cli migrate volumes --source root@old.example.com
```

Run the printed commands as root on the node that hosts the app. Then confirm each volume in the wizard's Volumes step. For apps brought in with `import platform`, enter the file and byte counts from both hosts on the checklist instead (`levelrail-cli migrate step APP data_verified --volume NAME=SRC_FILES:SRC_BYTES:DST_FILES:DST_BYTES`).

</Step>
<Step title="Verify">

```bash
levelrail-cli import apps --session appimp-123 --verify
```

Builds or pulls each app, waits for its health check, then stops it again until cutover. Test it with `curl --resolve app.example.com:443:NEW_IP https://app.example.com/healthz`.

</Step>
<Step title="Cut over">

```bash
levelrail-cli migrate cutover
levelrail-cli import cutover plan --session appimp-123 --app web
levelrail-cli import cutover run --session appimp-123 --app web --dry-run
levelrail-cli import cutover run --session appimp-123 --app web --confirm web
```

See [domains and TLS](#domains-and-tls) and [downtime](#downtime-window). Roll back with `levelrail-cli import cutover rollback --session appimp-123 --app web`.

</Step>
<Step title="Retire Coolify">

Stop Coolify only after a full traffic cycle and a checked backup. Download the receipt first: `levelrail-cli import apps --session appimp-123 --receipt`.

</Step>
</Steps>

## Databases

The copy streams `pg_dump`, `mysqldump` or `mariadb-dump`, `mongodump` or a Redis RDB snapshot from the source into the managed database, then compares row counts per table. It is safe to repeat: the restore replaces the target in one transaction. The source is only read.

Coolify databases often have no published port. The Levelrail node must reach the database host and port. Publish the port in Coolify temporarily (restricted by firewall to the new server) or use the `migrate server --container` path when the database container runs on the same node.

## Downtime window

| Item | Window |
| --- | --- |
| Stateless app | None expected with the old copy up and a low TTL |
| App with volumes | Last `rsync` pass, old app paused |
| Database | From pausing the old app until the new app is live: the final copy time plus a restart. Measure it with your first copy |

Pause or put the old Coolify app into maintenance mode before the final database copy, so no write lands after it.

## Domains and TLS

- Lower the TTL a day ahead. `levelrail-cli migrate cutover` returns `go`, `wait` (TTL too high), `no-go` (not healthy here) or `switched` per domain.
- The guided cutover switches a domain by the first method that applies: a connected Cloudflare or Route 53 record (previous value stored for rollback), the managed proxy route, or a record you set by hand followed by `import cutover confirm-dns`.
- It then checks the public name and rolls back on its own if that fails within `APP_CUTOVER_VERIFY_TIMEOUT` (5 minutes).
- Coolify's Traefik may own ports 80 and 443. Install Levelrail in [coexist mode](/behind-an-existing-proxy) first, or switch DNS to a new server.

## Common problems and fixes

| Problem | Fix |
| --- | --- |
| The import URL is refused | The control plane blocks private addresses by default. Set `APP_IMPORT_ALLOW_PRIVATE_NETWORKS=true` and pass `--allow-private` (same for loopback) |
| Secrets show as empty | Create a Coolify token with `read:sensitive` and plan again |
| A name is taken | Apply with `--collision suffix` (default) or skip it |
| Database copy says `failed` with differing tables | Run it again. A Redis count can differ if keys expire during the copy |
| `pg_dump` refuses the server | Create the target with the same or a newer major version |
| Image move fails on a remote node | Move it by hand: `docker save IMAGE \| ssh TARGET docker load` |
| Cutover reports `block` | The plan names the item and the fix. Fix it and run the dry run again |

## Choose to stay if

Coolify already does what you need and your team knows it. Levelrail is the younger project with fewer users, and its migration path has had little time on production data. If you cannot take a write freeze or test a rollback, wait. See the [comparison](/comparison) and [Coolify alternative](/coolify-alternative).
