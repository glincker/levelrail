---
title: "Migrate from Docker Compose"
sidebarTitle: "From Docker Compose"
description: Turn a docker-compose.yml into Levelrail apps and managed databases with a plan first, then copy data and switch DNS while the old stack keeps running.
"og:url": "https://docs.levelrail.com/migrate-from-docker-compose"
---

If you run a plain compose stack on a VM, there are two routes. Import the compose file to get one app per service and managed databases for the database images, or deploy the whole file as one compose app. Read the [migration overview](/migrate-to-levelrail) first for the flow and the downtime table.

## Prerequisites

- Levelrail installed on the new server and the CLI signed in (`levelrail-cli auth login`).
- The compose file, and for volumes and databases, shell access to the host that runs it.

## Pick a route

| Route | Use it when | Command |
| --- | --- | --- |
| Import the file | You want separate apps and managed databases, with backups and per-app settings | `levelrail-cli import apps --from compose --snapshot docker-compose.yml --plan` |
| Deploy as a compose app | You want the stack to stay one unit, as it runs today | `levelrail-cli apps deploy-compose NAME --file docker-compose.yml`, see [compose apps](/compose-apps) |
| Snapshot the running containers | The file is out of date or the images were built locally | `docker inspect $(docker ps -aq) > inspect.json`, then `--from docker --snapshot inspect.json` |

## What imports automatically (import the file)

| Item | Result |
| --- | --- |
| Services with an `image` | One app each: image, first container port, replicas 1 |
| Database-looking images | A managed database of that engine and major version |
| Environment | Variables from the service. Secret-looking names are stored as secrets |
| Volumes | Named volumes and bind mounts, listed on the app |
| Domains | Where the file's metadata names one |

## What needs a manual step

- **Services with only a `build:` block** are unsupported. Build and push an image, or create the app from its git repository.
- **Database and volume contents**, by the copy steps below.
- **Networks, `depends_on`, restart policy and `command` overrides** are not mapped. Set what matters on each app.
- **Hostnames between services.** Apps reach managed databases by connection variable, not by the compose service name. Rewrite them with the mapping table in the plan.

## What cannot be migrated

- Services without an image (build only).
- The compose network model: services do not share one bridge network by default.
- Anything the file does not say: running data, certificates and host proxy config.

## Steps

<Steps>
<Step title="Plan">

```bash
levelrail-cli import apps --from compose --snapshot docker-compose.yml --plan
```

Dashboard: Settings, Import from another platform, the Compose file tab. The content stays in memory for the session and secret values stay masked.

</Step>
<Step title="Stage">

```bash
levelrail-cli import apps --from compose --snapshot docker-compose.yml --apply
```

Apps are created stopped and not routed, and the session id is printed. Add `--only NAME` for a subset.

</Step>
<Step title="Copy database data">

```bash
export APP_MIGRATE_SOURCE_DB_PASSWORD=...
levelrail-cli migrate db-copy main --host old.example.com --port 5432 --user app --database app
```

The compose database must publish its port to the new server (restrict it by firewall), or run the dump yourself and use the [restore tools](/backups-and-storage).

</Step>
<Step title="Copy volumes">

```bash
levelrail-cli migrate volumes --source root@old.example.com
```

Run the printed `rsync` commands as root on the node that hosts the app.

</Step>
<Step title="Verify and switch">

```bash
levelrail-cli import apps --session ID --verify
levelrail-cli migrate cutover
levelrail-cli import cutover run --session ID --app NAME --dry-run
levelrail-cli import cutover run --session ID --app NAME --confirm NAME
```

Roll back with `levelrail-cli import cutover rollback --session ID --app NAME`.

</Step>
</Steps>

## Databases and the downtime window

Full dump and restore with row counts compared, repeatable, source read-only. No incremental sync. Stop the writers (`docker compose stop web`), run the final copy, start the app here, switch DNS. The window is the final copy time plus a restart. Stateless services with no volumes need no pause. See the [downtime table](/migrate-to-levelrail#what-stays-online-during-the-move).

## Domains and TLS

A plain compose stack usually sits behind your own reverse proxy. Point DNS at the new server or install Levelrail in [coexist mode](/behind-an-existing-proxy). Lower the TTL to 300 seconds a day ahead and run `levelrail-cli migrate cutover` for a go or no-go per domain. See [Domains and ingress](/domains-and-ingress).

## Common problems and fixes

| Problem | Fix |
| --- | --- |
| A service shows unsupported | It has no image. Build and push one |
| `--snapshot FILE is required` | Pass the compose file path, or `-` to read stdin |
| App cannot find its database | Use the connection variable from the managed database, not the old service name |
| Volume sizes unknown | The file has no sizes. The `rsync` comparison gives file counts and bytes |

## Choose to stay if

One VM and one compose file already do the job, and you do not need TLS automation, backups or rollback beyond what you built. A compose file is a fine deployment. Move when you want those things managed. See the [comparison](/comparison).
