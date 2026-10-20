---
title: "Migrate from Dokku"
sidebarTitle: "From Dokku"
description: Move Dokku apps to Levelrail by importing the env dump or a docker inspect snapshot. Live Dokku over SSH is not supported, so some steps are manual.
"og:url": "https://docs.levelrail.com/migrate-from-dokku"
---

Dokku has no HTTP API for Levelrail to read, and Levelrail does not log in to a Dokku host over SSH. So this is the least automated path. What you can import is the app's environment variables from a dump you create, or the running containers from a `docker inspect` snapshot. Everything else (image or repository, domains, ports, database data) you set or copy yourself. Read the [migration overview](/migrate-to-levelrail) first for the flow and the downtime table.

## Prerequisites

- Levelrail installed on the new server and the CLI signed in (`levelrail-cli auth login`).
- Shell access to the Dokku host, to run `dokku` commands and `docker inspect`.
- The app's git repository, or a registry image, to deploy from.

## What imports automatically

| Route | What it reads | Result |
| --- | --- | --- |
| Env dump (`--from dokku`) | `dokku config:export APP`, one app per run | One app with the variables. `DOKKU_*` and `GIT_REV` are dropped. Secret-looking names are masked in the plan and stored as secrets |
| Docker snapshot (`--from docker`) | `docker inspect` of the host's containers | Apps from containers: image, env, volumes and bind mounts, memory and CPU limits, health check. Database containers are recognized by image |

## What needs a manual step

- **Image or repository, port and domains** for the env dump route. Set them on the app after planning.
- **Domains** on the snapshot route: they are read from Traefik `Host` rules only, and Dokku uses nginx, so add them by hand.
- **Build.** Levelrail builds a Dockerfile with BuildKit, or detects the stack and builds with Railpack. It does not run Heroku buildpacks. See [Importing apps](/importing-apps) for detection.
- **Databases from Dokku plugins** (postgres, mysql, redis and others). The data is copied with the database copy step.
- **Persistent storage** from `dokku storage:mount`: copy the files by hand.
- **Procfile process types, cron, and plugin config** are not read.

## What cannot be migrated

- Live Dokku over SSH. There is no importer that connects to a Dokku host.
- Dokku's nginx config, certificates (Dokku's Let's Encrypt plugin), and deploy history. Certificates are issued here on first use.
- Buildpack-specific behavior. Check that the app builds with a Dockerfile or Railpack.

The snapshot route has not been rehearsed against a Dokku host. Read the plan before you apply it.

## Steps

<Steps>
<Step title="Dump the env on the Dokku host">

```bash
dokku config:export blog > dokku-env.txt
```

The file holds secrets. Delete it once the import is staged.

</Step>
<Step title="Plan and stage the app">

```bash
levelrail-cli import apps --from dokku --app-name blog --snapshot dokku-env.txt --plan
levelrail-cli import apps --from dokku --app-name blog --snapshot dokku-env.txt --apply
```

Dashboard: Settings, Import from another platform, the Dokku env dump tab. The content stays in memory for the session and secret-looking values stay masked.

</Step>
<Step title="Set the image or repository, port and domains">

Connect the repository (or a registry image) and set the port and domains on the app, then deploy. Or let Levelrail infer it: `levelrail-cli up` from a checkout of the repo. See [Getting started](/getting-started).

</Step>
<Step title="Alternative: snapshot the containers">

```bash
docker inspect $(docker ps -aq) > inspect.json
levelrail-cli import apps --from docker --snapshot inspect.json --plan
```

Images built on the Dokku host are flagged for the Move images step. See [Migrate from Coolify](/migrate-from-coolify#steps) for that step.

</Step>
<Step title="Copy databases">

Create a managed database of the same engine on Levelrail (same or newer major version). On the Dokku host, make the database reachable from the new server and read its credentials:

```bash
dokku postgres:info mydb
dokku postgres:expose mydb 5433
```

Then copy:

```bash
export APP_MIGRATE_SOURCE_DB_PASSWORD=...
levelrail-cli migrate db-copy main --host dokku.example.com --port 5433 --user postgres --database mydb
```

Restrict the exposed port by firewall to the new server, and run `dokku postgres:unexpose mydb` afterwards. If you would rather use Dokku's own tools, `dokku postgres:export` makes a dump you can restore with the [restore tools](/backups-and-storage).

</Step>
<Step title="Copy storage, check and switch">

Copy `dokku storage` directories by hand (for example with `rsync -aHAX`) into the new volume. Then:

```bash
levelrail-cli migrate cutover
levelrail-cli migrate cutover --verify
```

</Step>
</Steps>

## Databases and the downtime window

The copy is a full dump and restore with row counts compared and the source read-only. There is no incremental sync. Pause the Dokku app (`dokku ps:stop APP`), run the final copy, start the app here, switch DNS. The window is the final copy time plus a restart. A Dokku app that only serves from its image and has no database can move with no outage you would notice. See the [downtime table](/migrate-to-levelrail#what-stays-online-during-the-move).

## Domains and TLS

Dokku's nginx owns ports 80 and 443. Point DNS at a new server, or install Levelrail in [coexist mode](/behind-an-existing-proxy). Lower the TTL to 300 seconds a day ahead. Certificates are issued here on first use once DNS points here. See [Domains and ingress](/domains-and-ingress).

## Common problems and fixes

| Problem | Fix |
| --- | --- |
| `--app-name is required` | The dokku route needs the app name the dump belongs to |
| The app has no image to run | Connect the repository or set an image. The dump has only variables |
| Build fails without a Dockerfile | Railpack detection may not match a Heroku buildpack app. Add a Dockerfile |
| Database copy cannot connect | Expose the service port, open the firewall for the new server only |
| Variables missing | `DOKKU_*` and `GIT_REV` are dropped on purpose. Set the ones your app reads |

## Choose to stay if

You want Dokku's git push workflow with buildpacks and a very small footprint, and it already works. Levelrail builds from Dockerfiles or Railpack and has a beta push remote. See [Dokku alternative](/dokku-alternative) and the [comparison](/comparison).
