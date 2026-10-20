---
title: "Migrate from CapRover"
sidebarTitle: "From CapRover"
description: Import CapRover apps, env vars, domains and volumes into Levelrail with a read-only dry run. CapRover builds locally, so images need a repo or registry.
"og:url": "https://docs.levelrail.com/migrate-from-caprover"
---

CapRover apps import over its HTTP API, read-only. The main difference from the other sources is that CapRover builds images on its own host and does not expose them, so every app needs a source here: a git repository or a registry image. Read the [migration overview](/migrate-to-levelrail) first for the flow and the downtime table.

## Prerequisites

- Levelrail installed on the new server and the CLI signed in (`levelrail-cli auth login`).
- The CapRover login password. It is exchanged for a session token with `POST /api/v2/login` (the only non-GET request the importer makes).
- The CapRover URL reachable from the Levelrail control plane. For a private address, set `APP_IMPORT_ALLOW_PRIVATE_NETWORKS=true` and pass `--allow-private`.
- Each app's git repository, or a registry you can push its image to.

## What imports automatically

| Item | Result |
| --- | --- |
| Apps | One app per CapRover app, with instance count as replicas |
| Port | `containerHttpPort` (80 when unset) |
| Environment variables | Imported. Secret-looking names go through the encrypted secrets store |
| Domains | `<app>.<root domain>` unless not exposed, plus custom domains |
| Volumes | Volumes and bind mounts |
| Git source | The repository and branch when CapRover has an `appPushWebhook` |

## What needs a manual step

- **A source for every app.** Connect the repository (with credentials for a private one) and run a build, or push an image to a registry and set it as the source.
- **Volume contents**, by `rsync` commands you run.
- **Raw host port mappings** are not imported. Publish a port in the app settings if it must be reachable.
- **Health checks.** CapRover has none to import. Add a readiness path here.

## What cannot be migrated

- Images built on CapRover: there is no recoverable image through its API. Rebuild from source.
- Databases as a feature. CapRover runs databases as ordinary apps or one-click apps, so nothing maps to a managed database. Create a [managed database](/managing-databases) here and copy the data.
- Certificates, DNS records, deploy history, logs and metrics.

## Steps

<Steps>
<Step title="Dry run">

```bash
export APP_IMPORT_SOURCE_TOKEN=...   # the CapRover password
levelrail-cli import platform caprover --url https://captain.example.com --dry-run
```

</Step>
<Step title="Import">

```bash
levelrail-cli import platform caprover --url https://captain.example.com --only blog,api
```

Apps are created without a running image. The report names the repository and branch where one is known. For verdicts and one-click fixes, use `levelrail-cli migrate plan --platform caprover --url https://captain.example.com`, then add `--apply --only ID`.

</Step>
<Step title="Give each app a source and build">

Connect the git repository or set the registry image on the app, then deploy it. See [Importing apps](/importing-apps).

</Step>
<Step title="Move database data">

Create a managed database of the same engine, with the same or a newer major version. Then copy:

```bash
export APP_MIGRATE_SOURCE_DB_PASSWORD=...
levelrail-cli migrate db-copy main --host db.old.example.com --user app --database app
```

The CapRover database container must publish a port the Levelrail node can reach. Publish it temporarily and restrict it by firewall. Point the app at the managed database and set its connection variable.

</Step>
<Step title="Copy volumes, check, switch">

```bash
levelrail-cli migrate volumes --source root@old.example.com
levelrail-cli migrate cutover
levelrail-cli migrate cutover --verify
```

</Step>
</Steps>

## Databases and the downtime window

The database copy is a full dump and restore with row counts compared, repeatable, with the source read-only. There is no incremental sync. Pause the old app, run the final copy, start the app here, switch DNS. The window is the final copy time plus a restart. Apps without a database and without volumes can move with no outage you would notice. See the [downtime table](/migrate-to-levelrail#what-stays-online-during-the-move).

## Domains and TLS

CapRover runs nginx on ports 80 and 443. Point DNS at a new server, or install Levelrail in [coexist mode](/behind-an-existing-proxy). Lower the TTL to 300 seconds a day ahead and let `levelrail-cli migrate cutover` tell you when it is low enough. Certificates are issued here on first use once DNS points here. See [Domains and ingress](/domains-and-ingress).

## Common problems and fixes

| Problem | Fix |
| --- | --- |
| Login fails | Use the CapRover password, not a token, and check the URL scheme |
| An app has no image | Expected. Connect a repo or registry image and deploy |
| Domain not imported | The app was not exposed in CapRover. Add the domain by hand on the app |
| Database copy cannot connect | The source database port is not reachable from the Levelrail node |

## Choose to stay if

CapRover's one-click apps and simple Swarm setup already cover what you run, and you do not want to re-point apps at a git source. Levelrail has no recorded run against a live CapRover. See [CapRover alternative](/caprover-alternative) and the [comparison](/comparison).
