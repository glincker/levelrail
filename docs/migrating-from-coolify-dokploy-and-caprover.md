---
description: Import apps, databases, env vars, domains and volumes from Coolify, Dokploy or CapRover with a dry run first, then apply
---

# Migrating from Coolify, Dokploy, or CapRover

The platform importer reads a live Coolify, Dokploy or CapRover instance through its own HTTP API and creates the matching apps and databases here. It is read-only against the source: it never stops, changes or deletes anything there, and a test asserts it issues nothing but GET requests (CapRover's one login POST aside).

A migration of live apps is four steps, and the source keeps serving traffic through the first three:

1. [Import](#quick-start) the apps and databases (empty).
2. [Copy the data](#step-2-copy-database-data) in and let it verify row counts.
3. [Copy volumes](#step-3-copy-volumes), then run the app here in parallel with the source.
4. [Check the cutover](#step-4-cutover) and switch DNS one domain at a time.

You can run it three ways, all backed by the same two API routes:

- Dashboard: Settings, then Import from another platform.
- CLI: `levelrail-cli import platform coolify|dokploy|caprover --url URL`.
- API: `POST /api/v1/imports/platform/discover` and `/apply` (ability `write:sensitive`, source token in the request body only).

::: tip
Always start with a dry run. `--dry-run` (or the dashboard's Discover step) reads the source and shows exactly what would be created, per item, without creating anything.
:::

## Quick start

<Steps>
<Step title="Get a source credential">

What the token is, per platform:

<Tabs :items="['Coolify', 'Dokploy', 'CapRover']">
<Tab value="Coolify">

An API token (Keys and tokens). Use `read:sensitive` to get real env values. It is sent as `Authorization: Bearer`.

</Tab>
<Tab value="Dokploy">

An API key, sent in the `x-api-key` header.

</Tab>
<Tab value="CapRover">

The login password, exchanged for a session token via `POST /api/v2/login`.

</Tab>
</Tabs>

</Step>
<Step title="Dry run">

```bash
# token from the environment, not the command line
export APP_IMPORT_SOURCE_TOKEN=...            # or: --token-stdin
levelrail-cli import platform coolify --url https://coolify.example.com --dry-run
```

</Step>
<Step title="Apply, optionally for a subset">

```bash
levelrail-cli import platform coolify --url https://coolify.example.com --only web,api
```

</Step>
</Steps>

The token is sent to this control plane in the request body, used in memory for the duration of the request, and never stored, logged, written to the audit log, or returned in a response. Any error text is scrubbed of it. Prefer `APP_IMPORT_SOURCE_TOKEN` or `--token-stdin`: `--token` works but warns, because it lands in shell history and the process list.

## Flags

| Flag | Meaning |
| --- | --- |
| `--url` | Source platform base URL (required) |
| `--dry-run` | Report only, create nothing |
| `--only a,b` | Import only these source ids or names |
| `--collision suffix\|skip` | When a name is taken: rename with a numeric suffix (default) or skip |
| `--insecure-tls` | Skip TLS verification of the source (self-signed certificates) |
| `--allow-private`, `--allow-loopback` | Allow a private-network or loopback source, see below |
| `--token-stdin` | Read the token from the first line of stdin |
| `--json` | Print the report as JSON |

The command exits non-zero when any item failed, so a script can retry.

## Reaching a source on a private network

Sources usually live on your own network, often on a private address. The control plane refuses those by default, because an import URL is an outbound request an operator controls. Allowing it is an explicit, double opt-in:

1. The operator sets `APP_IMPORT_ALLOW_PRIVATE_NETWORKS=true` (or `APP_IMPORT_ALLOW_LOOPBACK=true` for a source on the same host) in the control plane's environment.
2. The request also sets `allow_private` (or `allow_loopback`), which the CLI exposes as `--allow-private` and `--allow-loopback` and the dashboard as a checkbox.

Either alone is rejected with a 400. Use of the opt-in is logged with the source host (never the token). Cloud metadata and link-local addresses (`169.254.0.0/16` including `169.254.169.254`, `fe80::/10`, `fd00:ec2::/32`, `100.100.100.200`) and unspecified or multicast addresses are blocked unconditionally, even with both opt-ins, because there is no legitimate reason to point an importer at an instance metadata service. The check runs on the resolved IP at dial time and after redirects, so a hostname that re-resolves to an internal address is still caught. Proxies are ignored for the same reason.

Other limits: 30 second request timeout, 8 MiB per response, bounded project and page walks, TLS verification on unless `--insecure-tls` is set, credentials embedded in the URL are rejected.

## No API token: claim containers from a Docker snapshot

When you cannot or do not want to hand over a Coolify, Dokploy or CapRover
token, the Docker host itself is enough. On the old server run:

```bash
docker inspect $(docker ps -aq) > inspect.json
```

Then plan and stage from the file (or paste it into the Docker snapshot tab
of the import wizard):

```bash
levelrail import apps --from docker --snapshot inspect.json --plan
levelrail import apps --from docker --snapshot inspect.json --apply
```

Read-only, nothing runs on or changes the old server. Platform containers
(the Coolify control plane, its proxy, Levelrail itself) are skipped, databases
are recognized by image, domains are read from Traefik Host rules, and
containers of the same app are grouped so an old exited copy never wins.
Things a snapshot cannot carry are flagged per app: an image built on the old
host (not in a registry) is marked with a `docker save | ssh | docker load`
hint, the Docker socket mount is dropped, and `COOLIFY_*` variables are left
out. The snapshot holds environment variables including secrets, so delete
the file once the session is staged.

## What the report tells you

Every discovered item gets one row with a status:

| Status | Meaning |
| --- | --- |
| Ready (`mapped`) | Maps cleanly, nothing for you to do |
| Needs attention (`needs-attention`) | Will be created, but with a caveat and a suggested manual step |
| Not supported (`unsupported`) | Cannot be imported, with the reason and what to do by hand |
| Already imported | Created by an earlier run, skipped |
| Skipped | Name already taken and `--collision skip` was set |
| Created, Failed | Outcomes after an apply |

Apply is idempotent and resumable. Every imported app carries the labels `import/source-id` (`<platform>:<source id>`), `import/source-platform` and `import/source-name`. A re-run finds apps by that label and skips them, so a partial failure is fixed by running the same command again: finished items are skipped and only the rest are attempted. Databases have no labels, so a re-run treats an existing database with the same name, engine and version as already imported.

## Mapping tables

### Apps

| Neutral field | Coolify | Dokploy | CapRover |
| --- | --- | --- | --- |
| Name | application name | application name | app name |
| Project | project name | project name | none |
| Image source | build pack `dockerimage`: image and tag | source type `docker`: `dockerImage` | not exposed |
| Git source | `git_repository`, `git_branch` | GitHub, Bitbucket, custom git URL, branch | `appPushWebhook.repoInfo` when set |
| Build method | `dockerfile` (path from base directory and dockerfile location), `static`, Nixpacks becomes Railpack | `dockerfile`, `static`, nixpacks and buildpacks become Railpack | Dockerfile assumed when a repo is known |
| Port | first of `ports_exposes` | port of the first domain | `containerHttpPort` (80 when unset) |
| Env vars | `/envs`, real value when the token allows, preview duplicates dropped | `env` text block parsed | `envVars` |
| Secret flag | hidden vars, plus secret-looking names | secret-looking names | secret-looking names |
| Domains | `fqdn`, comma separated | domain list | `<app>.<root domain>` unless not exposed, plus custom domains |
| Volumes | persistent storages | volume and bind mounts | volumes and bind mounts |
| Replicas | 1 | `replicas` | `instanceCount` |
| Health check | HTTP path, interval, timeout, retries | swarm health check when it is a curl or wget style HTTP probe | none |
| Resource limits | `limits_memory`, `limits_cpus` | `memoryLimit` bytes, `cpuLimit` nanoCPUs | none |
| Cron jobs | enabled scheduled tasks | not read | not applicable |

Secret-looking names are values whose key matches password, secret, token, key, credential, auth, DSN or a common database URL name. Those values go through the secrets manager (envelope encrypted, bound to the app), never into plain env. A secret variable that came back empty (Coolify redacts values without `read:sensitive`) is skipped and called out in the report.

### Databases

| Source | Engines mapped | Version |
| --- | --- | --- |
| Coolify | PostgreSQL, MySQL, MariaDB, MongoDB, Redis | major version from the image tag |
| Dokploy | PostgreSQL, MySQL, MariaDB, MongoDB, Redis | major version from the image tag |
| CapRover | none (databases are ordinary apps or one-click apps) | not applicable |

A database whose engine or version cannot be determined is reported as unsupported.

### Compose files and Dokku

The importer package can also map a plain Docker Compose file (one app per service with an image, database-looking images as managed databases) and a `dokku config:export` style environment dump, without contacting anything. These two file sources are not yet exposed through the API, CLI or dashboard, so today they are only reachable from Go code. Live Dokku over SSH is not supported.

## Step 2: copy database data

The import creates each database empty. The data copy step fills it from the live source and proves it arrived. It works for PostgreSQL, MySQL, MariaDB, MongoDB and Redis.

```bash
# password from the environment (or --password-stdin), never the command line
export APP_MIGRATE_SOURCE_DB_PASSWORD=...
levelrail-cli migrate db-copy main --host db.old-server.example.com --user app --database app
levelrail-cli migrate db-status
```

The same step is in the dashboard, under Settings, Import from another platform, "Copy database data".

How it works and what it guarantees:

- The control plane starts a short-lived helper container on the database's node, from the same engine image. It streams `pg_dump`, `mysqldump`/`mariadb-dump`, `mongodump` or an RDB snapshot from the source and pipes it straight into the managed database. Nothing is written to disk on the control plane and no `docker` CLI is involved.
- The source is only read. Source credentials are held in memory for the copy and passed to the helper as environment variables. They are never stored, logged or returned, and any error text is scrubbed of the password.
- After the restore, row counts per table (collection counts for MongoDB, key count for Redis) are compared between source and target. The status is `verified` only when every source table matches, otherwise `failed` with the tables that differ.
- It is idempotent. The restore replaces the target's contents in one transaction, so a failed or interrupted copy is fixed by running the same command again. A copy that was started and never finished (for example the control plane restarted) can be retried after `APP_MIGRATE_COPY_TIMEOUT` (default `2h`).
- Per-database status is `pending`, `copying`, `verified`, `failed` (with the reason) or `unsupported` (with the next step).

Practical notes:

- Create the target with the same or a newer major version than the source. `pg_dump` refuses to read a server newer than itself.
- The dump is a point in time. For a database that keeps taking writes, copy once now to prove the path, then again right before the DNS switch (see [cutover](#step-4-cutover)), ideally with the source app stopped or read-only for that last run.
- The source host must be reachable from the node that runs the database. Link-local and metadata addresses are refused.
- ClickHouse, KeyDB and Dragonfly are reported `unsupported`. Take a dump with the source's own tools and restore it with the backup and restore tools.
- Redis is copied as an RDB snapshot and the target restarts to load it. Keys that expire during the copy can make the key counts differ slightly, in which case the status is `failed` with both numbers shown. Re-run it.

## Migrating a whole database server (Migration hub)

When the source is a database server with many databases, use the Migration
hub under Settings, Import from another platform. It is one guided run:
Connect, Inventory, Preflight, Copy, Verify, Cutover. The plan is stored on
the control plane, so you can leave and come back.

- **Connect**: host, port, user and password once. The password is held in
  memory for the session (`APP_MIGRATE_HUB_PASSWORD_TTL`, default 2h) and is
  never stored or logged. Postgres, MySQL, MariaDB and MongoDB are supported.
- **Inventory**: every database with size, table count and extensions.
  Choose which to copy and edit the name of each new managed database.
- **Preflight**: nothing is written until it passes. It checks extensions
  against what the managed image provides (an extension no image ships blocks
  that database, it is never dropped silently), major version, free disk with a
  margin (`APP_MIGRATE_DISK_MARGIN`, default 1.5), name collisions and an
  estimated copy time (`APP_MIGRATE_ASSUMED_MBPS`).
- **Copy**: one managed database per selected source database, copied with
  bounded concurrency (`APP_MIGRATE_HUB_CONCURRENCY`, default 2). One failure
  never stops the others and repeating a copy is safe.
- **Verify**: exact per-table row counts. A receipt (JSON, no secrets) records
  each verified database and that the source was only read.
- **Cutover**: the connection details and environment variables each app
  needs. The secret is hidden until you click reveal, which is audited.

The source is only read: sessions run with `default_transaction_read_only=on`
and only dump tools and catalog queries.

The same flow from the CLI:

```sh
levelrail-cli migrate server --engine postgres --host db.old.example.com --user admin --password-stdin --list
levelrail-cli migrate server --engine postgres --host db.old.example.com --user admin --password-stdin --plan
levelrail-cli migrate server --engine postgres --host db.old.example.com --user admin --password-stdin --apply
```

The MCP server exposes `get_migration_plan` read-only. Starting a copy needs
the source password and is deliberately not available to agents.

## Step 3: copy volumes

Volume contents are not moved by the control plane, because that needs root on the source host. The CLI and the dashboard print the exact command per volume instead:

```bash
levelrail-cli migrate volumes --source root@old-server.example.com
```

Run the printed commands as root on the node that hosts the app. Each one creates the volume if needed and runs `rsync -aHAX --numeric-ids --delete` over SSH. Run it once while the source is live to move the bulk, then again right before the DNS switch to pick up the last changes. A bind mount uses the same host path on both sides.

## Step 4: cutover

Do not point DNS at this platform until the app is healthy here. The cutover check reads app status and public DNS and gives a go or no-go per domain. It changes nothing.

```bash
levelrail-cli migrate cutover                 # before the switch
levelrail-cli migrate cutover --verify        # after the switch
```

The dashboard has the same two buttons under "Cutover check". The command exits non-zero unless every domain is `go` or `switched`, so a script can gate on it.

For each domain of each imported app it checks:

| Check | What it tells you |
| --- | --- |
| Readiness | The app's readiness probe passes here. Not ready is a hard no-go. |
| DNS TTL | How long clients keep the old address. Above 300 seconds the verdict is `wait`, with the exact TTL to set and how long to wait. |
| DNS target | Where the domain resolves now, and whether it already points at this node. |

The verdicts are:

| Verdict | Meaning |
| --- | --- |
| `go` | Healthy here, TTL is low. Make the record change shown. |
| `wait` | Healthy, but the TTL is too high. Lower it first and wait it out. |
| `no-go` | The app is not healthy here yet. Fix that first. |
| `switched` | The domain already resolves to this node. |

### DNS TTL guidance

1. A day or more before the switch, lower the TTL of each domain's record to 300 seconds (60 if your provider allows) at your DNS provider. Leave the value alone for now.
2. Wait at least the old TTL. If it was 3600, wait an hour. Run `migrate cutover` again: the TTL check turns to pass when the resolvers see the low value. The TTL shown is read from the domain's authoritative servers, and falls back to a caching resolver (marked as such) if they cannot be reached.
3. Only then change the record. The check prints it exactly, for example `set A app.example.com to 203.0.113.10 (TTL 300)`. If the name currently holds a CNAME, remove it first: a name cannot hold a CNAME and an address.
4. After the move is verified and stable, raise the TTL back up.

### Parallel run

The source keeps serving until you change DNS, and keeps serving the clients whose resolvers still cache the old address after you do. Use that:

1. Run the imported app here with the data copied in and test it directly, for example `curl --resolve app.example.com:443:203.0.113.10 https://app.example.com/healthz`.
2. Do the final data and volume copy, then switch one domain at a time.
3. Run `migrate cutover --verify`. It checks, against this node with the domain as the TLS server name, that a trusted certificate is served and that the app's health path answers 200, and separately that public DNS resolves to this node. The certificate is issued on first use, so allow a minute after the first request.
4. To roll back, point the record at the source again. Because the TTL is low, that takes minutes. Keep the source running for at least a full traffic cycle before stopping it.

Writes accepted by the source after the final copy and before clients move are not carried over. For databases with steady writes, put the source app in maintenance mode for the last copy and the switch.

## What does not migrate

- Databases the copy step does not support (ClickHouse, KeyDB, Dragonfly), and any database you did not run the copy for. They stay empty.
- Volume contents are only moved by running the commands from step 3. Bind mounts need the same host path on the target node and the root ability; without root they are skipped and reported.
- Git-built apps do not build automatically. They are created with a pending image so nothing runs, and the report names the repository and branch. Connect the repository (with credentials for a private one) and run a build.
- CapRover apps have no recoverable image, because CapRover builds locally. Connect a repository or push an image to a registry, then set it as the source.
- Compose stacks, Coolify one-click services and Dokploy compose apps. They are multi-service and are reported as unsupported, deploy their compose file with the compose deploy command.
- Dokploy apps uploaded as an archive (`drop`), and file mounts. Unsupported.
- Health checks that are shell commands, pre and post deploy commands, extra ports and raw host port mappings, Coolify preview variables. Called out as needs attention where relevant.
- TLS certificates, DNS records, deploy history, logs and metrics. Certificates are issued here on first use. Re-point DNS only after the cutover check says `go`.
- Cron jobs are reported with the schedule and command so you can recreate them as scheduled tasks.

## Rollback

The importer only ever adds resources here and never touches the source, so the source keeps running throughout. To undo an import:

1. Find what it created: the apply report lists every created app and database by name, and each app carries the label `import/source-id` (visible in `levelrail-cli apps get <name> --json`).
2. Delete those apps (`levelrail-cli apps delete <name>`) and the databases the report listed.
3. Delete any projects the import created if they are empty.

Because nothing on the source changed, no source-side rollback is needed. Keep DNS pointing at the source until the cutover check says `go`. If you already switched a domain, point its record back at the source (the check printed the old value as "replaces").

## Manual checklist after an import

- [ ] Read every "needs attention" and "not supported" row and its suggested step.
- [ ] Run `migrate db-copy` for every database and confirm each is `verified`.
- [ ] Run the `migrate volumes` commands and copy volume data.
- [ ] Update each app's connection settings to the managed database.
- [ ] Connect repositories and run builds for git-based apps.
- [ ] Confirm secrets landed (the Secrets card on each app) and re-enter any that were skipped.
- [ ] Lower DNS TTLs to 300 and wait out the old value.
- [ ] Run `migrate cutover` and switch only domains that are `go`.
- [ ] Run `migrate cutover --verify` after each switch.
- [ ] Re-add health checks and cron jobs the report listed.

## The older `migrate` commands

`levelrail-cli migrate coolify|dokploy|caprover` predates the importer. It reads the same sources but writes `app.yaml` files (or creates apps one by one with `--apply`) instead of talking to the server-side importer, and it does not handle databases, volumes as resources, idempotent re-runs or the network opt-in. Use `import platform` for new work.

## See also

- [Getting started](getting-started.md): deploying your first app
- [App spec reference](app-spec-reference.md): full app.yaml schema
- [Backups and storage](backups-and-storage.md): dump and restore tools for engines the copy step does not cover
- [Comparison](comparison.md): architectural differences between platforms
- [Coolify alternative](coolify-alternative.md) and [Dokploy alternative](dokploy-alternative.md): why people switch

## Guided app import from Coolify

Settings, Import from another platform, "Move apps from Coolify" walks every application through plan, stage, verify, volumes and cutover. The CLI mirrors it:

```
levelrail-cli import apps --from coolify --url https://coolify.example.com --token-stdin --plan
levelrail-cli import apps --from coolify --url https://coolify.example.com --token-stdin --apply --only web --map OLD_DB_HOST=NEW_DB_HOST
levelrail-cli import apps --session ID --verify
levelrail-cli import apps --session ID --receipt
```

What it guarantees:

- Coolify is only read. Every request to it is a GET, and the token lives in memory for the session and in the request body only.
- Staged apps are created stopped and not routed. Variables and secrets are imported encrypted, domains are held back until you enable routing.
- Each app gets a verdict (ready, ready with notes, needs attention, unsupported) with the reason and next action. Docker Compose apps and one-click services are listed as unsupported with the manual path.
- Old database hostnames are only rewritten through a mapping table you confirm, with a masked before and after diff. Suggestions come from databases you already moved. When the target is a managed database, the staged app also gets a connection variable so it joins that database's network.
- Verify builds or pulls the app, waits for its own health check and stops it again until cutover.
- Volume data is copied by you with a generated rsync command per volume, and you confirm it. Volume sizes are shown only when Coolify reports them.
- Rollback removes only apps carrying this session's label. Volumes are kept on disk.
- The receipt is a JSON download with no secret values. The MCP tool `get_app_import_plan` is read-only.

If the token lacks the `read:sensitive` ability, secret values come back empty and the app is flagged so you can set them by hand.
