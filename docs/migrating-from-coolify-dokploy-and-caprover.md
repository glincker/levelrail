---
description: Import apps, databases, env vars, domains and volumes from Coolify, Dokploy or CapRover with a dry run first, then apply
---

# Migrating from Coolify, Dokploy, or CapRover

The platform importer reads a live Coolify, Dokploy or CapRover instance through its own HTTP API and creates the matching apps and databases here. It is read-only against the source: it never stops, changes or deletes anything there, and a test asserts it issues nothing but GET requests (CapRover's one login POST aside).

You can run it three ways, all backed by the same two API routes:

- Dashboard: Settings, then Import from another platform.
- CLI: `levelrail-cli import platform coolify|dokploy|caprover --url URL`.
- API: `POST /api/v1/imports/platform/discover` and `/apply` (ability `write:sensitive`, source token in the request body only).

::: tip
Always start with a dry run. `--dry-run` (or the dashboard's Discover step) reads the source and shows exactly what would be created, per item, without creating anything.
:::

## Quick start

```bash
# token from the environment, not the command line
export APP_IMPORT_SOURCE_TOKEN=...            # or: --token-stdin
levelrail-cli import platform coolify --url https://coolify.example.com --dry-run

# happy with the report? run it for real, optionally for a subset
levelrail-cli import platform coolify --url https://coolify.example.com --only web,api
```

What the token is, per platform:

| Platform | Credential | How it is sent to the source |
| --- | --- | --- |
| Coolify | API token (Keys and tokens). Use `read:sensitive` to get real env values | `Authorization: Bearer` |
| Dokploy | API key | `x-api-key` header |
| CapRover | Login password | exchanged for a session token via `POST /api/v2/login` |

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

## What does not migrate

- Database contents. Databases are created empty. Dump each source database and restore it with the backup and restore tools, then update the app's connection settings. The report and the dashboard both say so.
- Volume contents. Volumes are created empty, copy the data across before starting the app. Bind mounts need the same host path on the target node and the root ability; without root they are skipped and reported.
- Git-built apps do not build automatically. They are created with a pending image so nothing runs, and the report names the repository and branch. Connect the repository (with credentials for a private one) and run a build.
- CapRover apps have no recoverable image, because CapRover builds locally. Connect a repository or push an image to a registry, then set it as the source.
- Compose stacks, Coolify one-click services and Dokploy compose apps. They are multi-service and are reported as unsupported, deploy their compose file with the compose deploy command.
- Dokploy apps uploaded as an archive (`drop`), and file mounts. Unsupported.
- Health checks that are shell commands, pre and post deploy commands, extra ports and raw host port mappings, Coolify preview variables. Called out as needs attention where relevant.
- TLS certificates, DNS records, deploy history, logs and metrics. Re-point DNS only after the app is healthy here.
- Cron jobs are reported with the schedule and command so you can recreate them as scheduled tasks.

## Rollback

The importer only ever adds resources here and never touches the source, so the source keeps running throughout. To undo an import:

1. Find what it created: the apply report lists every created app and database by name, and each app carries the label `import/source-id` (visible in `levelrail-cli apps get <name> --json`).
2. Delete those apps (`levelrail-cli apps delete <name>`) and the databases the report listed.
3. Delete any projects the import created if they are empty.

Because nothing on the source changed, no source-side rollback is needed. Keep DNS pointing at the source until the imported app is healthy.

## Manual checklist after an import

- [ ] Read every "needs attention" and "not supported" row and its suggested step.
- [ ] Restore database dumps and copy volume data.
- [ ] Connect repositories and run builds for git-based apps.
- [ ] Confirm secrets landed (the Secrets card on each app) and re-enter any that were skipped.
- [ ] Check domains, then re-point DNS.
- [ ] Re-add health checks and cron jobs the report listed.

## The older `migrate` commands

`levelrail-cli migrate coolify|dokploy|caprover` predates the importer. It reads the same sources but writes `app.yaml` files (or creates apps one by one with `--apply`) instead of talking to the server-side importer, and it does not handle databases, volumes as resources, idempotent re-runs or the network opt-in. Use `import platform` for new work.

## See also

- [getting-started.md](getting-started.md) - deploying your first app
- [app-spec-reference.md](app-spec-reference.md) - full app.yaml schema
- [backups-and-storage.md](backups-and-storage.md) - dump and restore tools for moving data
- [comparison.md](comparison.md) - architectural differences between platforms
