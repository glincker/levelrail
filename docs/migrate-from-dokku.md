---
title: "Migrate from Dokku"
sidebarTitle: "From Dokku"
description: Move Dokku apps to Levelrail by reading the Dokku host's Docker over SSH, or from an env dump as the fallback. Includes a Dokku command map.
"og:url": "https://docs.levelrail.com/migrate-from-dokku"
---

A Dokku host is a Docker host. Its apps run as plain containers, so Levelrail can read them over SSH without installing anything on the old server: the control plane opens an SSH login, uses it only to reach the Docker socket, and sends read-only requests. That is the first route on this page. If you cannot give Levelrail an SSH login, you can import one app's environment variables from a `dokku config:export` dump instead. Read the [migration overview](/migrate-to-levelrail) first for the flow and the downtime table.

## Prerequisites

- Levelrail installed on the new server and the CLI signed in (`levelrail-cli auth login`).
- An SSH login on the Dokku host whose user can use the Docker socket (member of the `docker` group, or root). See [Migrate over SSH](/migrate-over-ssh) for what is read and what is never done.
- The app's git repository, or a registry image, to deploy from.

## What moves

| Route | What it reads | Result |
| --- | --- | --- |
| Live SSH (`--from docker-ssh`) | The running containers through the Docker socket | Apps from containers: image, env, ports, volumes and mounts, limits, health check. Host built images and named volumes can be streamed through the same tunnel. Database containers are recognized by image. When containers carry Dokku's app-name and process-type labels, they are grouped into one app named after the Dokku app, with one app per process type (the web process takes the Dokku app name, others are named app-process, and scaled containers become replicas). Domains and per-app config are not readable over the socket, so the report flags them for you to copy |
| Env dump (`--from dokku`) | `dokku config:export APP`, one app per run | One app with the variables. `DOKKU_*` and `GIT_REV` are dropped. Secret-looking names are masked in the plan and stored as secrets |
| Docker snapshot (`--from docker`) | `docker inspect` output you save yourself | Same mapping as the SSH route, from a file |

## What needs a manual step

- **Domains.** They live in Dokku's stored config, which is not read. Add them after the plan with `levelrail-cli apps domains add`.
- **Build.** Levelrail builds a Dockerfile with BuildKit, or detects the stack and builds with Railpack. It does not run Heroku buildpacks. See [Buildpack apps](/importing-apps#buildpack-apps).
- **Procfile process types.** The import reads containers, not the Procfile. Check each service after the plan. `levelrail-cli import config` reads a Procfile and `app.json` offline and proposes an `app.yaml`.
- **Persistent storage** from `dokku storage:mount`. Named volumes can be copied through the tunnel. Bind mounts and stopped containers use the printed `rsync` commands. See [Migrate over SSH](/migrate-over-ssh#images-and-volumes).
- **Datastores** from Dokku plugins (postgres, mysql, redis and others) are copied with the database copy step below.
- **Cron and plugin config** are not read.

## What does not move

- Dokku's proxy config, certificates from its Let's Encrypt plugin, and deploy history. Certificates are issued here on first use once DNS points here.
- Buildpack-specific behavior. Check that the app builds with a Dockerfile or Railpack before the cutover.

The Dokku routes have not been rehearsed against a live Dokku host. Read the plan before you apply it, and keep the Dokku app running until you have verified the copy.

## Steps

<Steps>
<Step title="Test the SSH connection">

```bash
levelrail-cli import apps --from docker-ssh --ssh root@dokku.example.com --ssh-key ~/.ssh/old_key --test-connection
```

With no `--fingerprint` it lists the host keys the server presents. Compare one with what the Dokku host reports, then pass it with `--fingerprint SHA256:...`. The test shows the container, volume and image counts and changes nothing. Use a key made for this move and remove it from `authorized_keys` afterwards.

</Step>
<Step title="Plan and stage the apps">

```bash
levelrail-cli import apps --from docker-ssh --ssh root@dokku.example.com --ssh-key ~/.ssh/old_key \
  --fingerprint SHA256:... --plan
levelrail-cli import apps --from docker-ssh --ssh root@dokku.example.com --ssh-key ~/.ssh/old_key \
  --fingerprint SHA256:... --apply
```

The plan says why it made each choice. Apply creates the apps stopped and not routed, with env and secrets stored encrypted. The dashboard has the same flow under Migrate, Import apps, Connect over SSH.

</Step>
<Step title="Move images and volumes">

```bash
levelrail-cli import apps --session ID --transfer-images
levelrail-cli import apps --session ID --copy-volumes
levelrail-cli import apps --session ID --verify
```

Images that Dokku built on the host are streamed through the tunnel and checked by image ID. Run the volume copy again right before the DNS switch to catch the last changes.

</Step>
<Step title="Set the domains">

Add each domain to the staged app, then check it before you change DNS:

```bash
levelrail-cli apps domains add blog blog.example.com
levelrail-cli domains check blog blog.example.com
```

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
<Step title="Check and switch">

```bash
levelrail-cli migrate cutover
levelrail-cli migrate cutover --verify
```

</Step>
</Steps>

## Fallback: env dump for one app

Use this when you have no SSH login to give, or only want the variables. It brings in the environment and nothing else: set the image or repository, port and domains on the app yourself.

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
</Steps>

## Dokku command map

The commands you used every day, and where they went. Levelrail commands are written without the `levelrail-cli` prefix. Rows marked **from v0.9.5** are being added on the v0.9.5 release branch and are not in earlier versions. There is no `dokku` alias in the CLI on purpose: Levelrail calls an API with permissions and approvals, and a Dokku-shaped alias would hide that.

| Dokku | Levelrail | Notes |
| --- | --- | --- |
| `apps:create`, `apps:destroy` | `apps create`, `apps delete` | `up` creates the app for you on first run |
| `apps:clone` | `apps clone NAME NEW` | |
| `apps:rename` | Not available yet | Clone the app, then delete the old one |
| `apps:report`, `ps:report` | `apps get`, `apps status` | |
| `config:set` | `env set APP KEY=VALUE --apply` | Different on purpose: without `--apply` the change stays pending, so you can batch several. Use `apps secrets set` for secret values |
| `config:unset` | `env unset APP KEY` | |
| `config:get`, `config:show`, `config:export` | `apps env get`, `apps env export` | Secret values stay masked unless you ask with `--reveal` |
| `config:import` | `apps env import APP --file .env` | |
| `domains:add`, `domains:remove`, `domains:report` | `apps domains add`, `apps domains remove`, `apps domains list` | `--replace` swaps the whole list. DNS records can be created for you |
| `certs:add` | `domains tls-cert set APP DOMAIN --cert-file F --key-file F` | A `domains cert upload` form is **from v0.9.5** |
| `letsencrypt:enable`, auto-renew | Nothing to run | Certificates are issued on first use and renewed. `domains tls-cert renew` forces it |
| `ps:restart`, `ps:stop`, `ps:start` | `apps restart`, `apps stop`, `apps start` | One command acts on the whole app |
| `ps:rebuild` | `apps builds trigger APP --repo URL --ref REF` | Or push again |
| `ps:scale web=2 worker=1` | `apps scale APP web=2 worker=1` or `apps scale APP --replicas N` | The `service=count` pairs set one count per service of a multi-service app, can not be combined with `--replicas`, and a failed update sets the earlier services back. The pairs form is **from v0.9.5** |
| `run APP cmd` | `apps run APP -- cmd` or `jobs run APP --command "cmd" --wait` | Runs once in a fresh container with the app's env and waits. `--service` picks a service. `apps run` is **from v0.9.5** |
| `enter APP`, `ps:exec` | `apps exec APP -- cmd`, `apps console APP` | |
| `logs APP -t` | `apps logs APP --follow` | `--tail N` and `--since 1h` also work. `--service` to pick one service is **from v0.9.5** |
| `storage:mount` | `apps volumes attach APP --name N --path P` | Named volumes. A host path needs root approval and a spec file |
| `docker-options:add` | `apps runtime set APP --ulimit nofile=65536 --tmpfs /tmp:size=64m --init --dns IP` | Runtime options: tmpfs, ulimits, init, stop timeout, extra hosts, DNS, sysctls and pids limit. A change recreates the container. See [Runtime options](/runtime-options). Not available: `cap_add`, `privileged`, devices, `security_opt`, host namespaces and host bind mounts, which are refused with the key named |
| `nginx:set client-max-body-size`, `proxy-read-timeout` | `domains limits set APP DOMAIN --max-body 100MiB --read-timeout 120 --write-timeout 120` | Per domain, bounded by instance ceilings. See [Domain traffic controls](/domain-traffic-controls) |
| `network:*` | `apps connect APP DATABASE` and private networks | Different on purpose: services reach each other by name on a private network |
| `maintenance:on`, `maintenance:off` | `domains maintenance set`, `domains maintenance clear` | Per domain, and the container keeps running |
| `apps:lock` | `apps freeze set` | A freeze window with a reason, instead of a permanent lock |
| `git:from-image` | `deploy APP --image IMAGE` | |
| `git push dokku` | `git remote add APP --apply`, then a normal push | `--apply` sets the remote in your local repository. Beta, see [Deploy with git push](/git-push-deploy) |
| `postgres:create` | `databases create --name N --engine postgres --version V` | Same for the other engines in the catalog |
| `postgres:link`, `postgres:unlink` | `apps connect APP DATABASE`, `apps disconnect APP ENV_VAR` | Injects the connection value as an env var |
| `postgres:info`, `postgres:logs` | `databases get`, `databases logs` | |
| `postgres:backup`, `postgres:backup-schedule` | `backups trigger DB`, `backups schedule set DB --target ID --cron EXPR` | |
| `postgres:export`, `postgres:import` | `backups download`, `backups restore`, `databases import-hosted` | Different on purpose: backups go through a backup target. There is no stdin and stdout dump yet |
| `postgres:clone` | `databases branch create SRC NEW --from-backup ID` | `databases clone SRC NEW` is **from v0.9.5** |
| `postgres:expose`, `postgres:unexpose` | `databases public-access set`, `databases public-access clear` | |
| `postgres:connect` | `databases query NAME --sql "..."` | No interactive shell yet |
| `plugin:install` | Not available | There is no plugin system. Templates, the API and MCP are the extension points |
| `events`, `report` | The audit log and the app timeline | |

## Datastore coverage

Each Dokku datastore plugin maps to a managed database engine, a catalog template you run as an app, or nothing. A managed engine gets generated credentials, backups, restore and the database copy step. A catalog template is an ordinary app with its own volume: move its data with the volume copy in [Migrate over SSH](/migrate-over-ssh#images-and-volumes) and back it up as app storage.

| Datastore | Offered as | Name |
| --- | --- | --- |
| Postgres, MySQL, MariaDB, MongoDB, Redis | Managed engine | `postgres`, `mysql`, `mariadb`, `mongodb`, `redis` |
| Memcached, ClickHouse, Meilisearch, Typesense | Managed engine | `memcached`, `clickhouse`, `meilisearch`, `typesense` |
| CouchDB, Elasticsearch, OpenSearch, Solr | Catalog template | `couchdb`, `elasticsearch`, `opensearch`, `solr` |
| RabbitMQ, NATS | Catalog template | `rabbitmq`, `nats` |
| InfluxDB, Neo4j | Catalog template | `influxdb`, `neo4j` |
| RethinkDB, OrientDB, Graphite, Pushpin | Not offered | none |

The database copy step handles managed engines only. Catalog templates have no dump and restore step, so stop the source service before copying its volume. `GET /api/v1/database-engines` is the live source for engines.

## Databases and the downtime window

The copy is a full dump and restore with row counts compared and the source read-only. There is no incremental sync. Pause the Dokku app (`dokku ps:stop APP`), run the final copy, start the app here, switch DNS. The window is the final copy time plus a restart. A Dokku app that only serves from its image and has no database can move with no outage you would notice. See the [downtime table](/migrate-to-levelrail#what-stays-online-during-the-move).

## Domains and TLS

Dokku's own proxy owns ports 80 and 443. Point DNS at a new server, or install Levelrail in [coexist mode](/behind-an-existing-proxy). Lower the TTL to 300 seconds a day ahead. Certificates are issued here on first use once DNS points here. See [Domains and ingress](/domains-and-ingress).

## Common problems and fixes

| Problem | Fix |
| --- | --- |
| `--app-name is required` | The env dump route needs the app name the dump belongs to |
| The app has no image to run | Connect the repository or set an image. The dump has only variables |
| Build fails without a Dockerfile | Railpack detection may not match a Heroku buildpack app. Add a Dockerfile |
| Database copy cannot connect | Expose the service port, open the firewall for the new server only |
| Variables missing | `DOKKU_*` and `GIT_REV` are dropped on purpose in the env dump route. Set the ones your app reads |
| SSH test fails | See the failure table in [Migrate over SSH](/migrate-over-ssh#failures-and-their-fixes) |
| Domains are missing after the import | They are not read from Dokku. Add them with `apps domains add` |

## Verify and roll back

Run `levelrail-cli migrate cutover --verify` after you switch DNS. It checks the certificate, the health path and DNS.

The copy only reads the old server, so nothing there is changed. To roll back, restart the old app if you paused it and point DNS back at the old server.

## Choose to stay if

You want Dokku's git push workflow with buildpacks and a very small footprint, and it already works. Levelrail builds from Dockerfiles or Railpack and has a beta push remote. See [Dokku alternative](/dokku-alternative) and the [comparison](/comparison).
