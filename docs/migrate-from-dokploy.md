---
title: "Migrate from Dokploy"
sidebarTitle: "From Dokploy"
description: Import Dokploy applications, databases, env vars and domains into Levelrail with a read-only dry run, then copy data and switch DNS while Dokploy keeps serving.
"og:url": "https://docs.levelrail.com/migrate-from-dokploy"
---

Dokploy apps and databases import over its HTTP API, read-only. Data, volumes and traffic are moved by the same steps as every other source. Read the [migration overview](/migrate-to-levelrail) first for the flow and the downtime table.

## Prerequisites

- Levelrail installed on the new server and the CLI signed in (`levelrail-cli auth login`).
- A Dokploy API key. It is sent in the `x-api-key` header.
- The Dokploy URL reachable from the Levelrail control plane. For a private address, set `APP_IMPORT_ALLOW_PRIVATE_NETWORKS=true` on the control plane and pass `--allow-private`.
- A database user per Dokploy database, and a route from the Levelrail node to its port.

## What imports automatically

| Item | Result |
| --- | --- |
| Applications | Image apps (`docker` source), git apps (GitHub, Bitbucket, custom git URL, branch), build method (`dockerfile`, `static`, nixpacks and buildpacks become Railpack) |
| Port | The port of the first domain |
| Environment variables | The `env` text block parsed. Secret-looking names go through the encrypted secrets store |
| Domains | The domain list |
| Volumes | Volume and bind mounts |
| Replicas, memory and CPU limits | Imported |
| Health check | Only when it is a curl or wget style HTTP probe |
| Databases | PostgreSQL, MySQL, MariaDB, MongoDB and Redis, created empty with the major version from the image tag |

## What needs a manual step

- Database contents and volume contents (below).
- Git apps do not build by themselves: connect the repository and run a build.
- Health checks that are shell commands, extra ports, pre and post deploy commands.
- Dokploy scheduled jobs are not read. Recreate them as scheduled tasks.

## What cannot be migrated

- Dokploy [compose apps](/compose-apps). They are listed as unsupported. Deploy the compose file with `levelrail-cli apps deploy-compose`.
- Apps uploaded as an archive (`drop`) and file mounts.
- Swarm services as such: Levelrail runs plain Docker containers, so Swarm placement and overlay networking do not carry over.
- Certificates, DNS records, deploy history, logs and metrics.

## Steps

<Steps>
<Step title="Dry run">

Dashboard: Settings, Import from another platform. CLI:

```bash
export APP_IMPORT_SOURCE_TOKEN=...
levelrail-cli import platform dokploy --url https://dokploy.example.com --dry-run
```

You get one row per item: ready, needs attention (with the manual step) or not supported (with the reason).

</Step>
<Step title="Import, optionally a subset">

```bash
levelrail-cli import platform dokploy --url https://dokploy.example.com --only web,api
```

Apps and databases are created empty. Re-running skips what exists. Imported apps carry the labels `import/source-id`, `import/source-platform` and `import/source-name`.

For a plan with verdicts and one-click fixes (name, port, hold domains), use the wizard: `levelrail-cli migrate plan --platform dokploy --url https://dokploy.example.com`, then add `--apply --only ID`.

</Step>
<Step title="Copy databases">

```bash
export APP_MIGRATE_SOURCE_DB_PASSWORD=...
levelrail-cli migrate db-copy main --host db.old.example.com --user app --database app
levelrail-cli migrate db-status
```

</Step>
<Step title="Copy volumes">

```bash
levelrail-cli migrate volumes --source root@old.example.com
```

Run the printed `rsync` commands as root on the node that hosts the app. Run once live, then once more at cutover.

</Step>
<Step title="Check and switch">

```bash
levelrail-cli migrate cutover
levelrail-cli migrate cutover --verify
```

The first command gives `go`, `wait`, `no-go` or `switched` per domain and prints the exact record to set. The second checks the certificate, the health path and DNS after you switch.

</Step>
</Steps>

<Note>
The dashboard's guided app wizard (stage, move images, verify, guided cutover) connects to Coolify. The CLI accepts `import apps --from dokploy`, but it has been exercised mostly with Coolify. The steps above use the importer that is built for Dokploy.
</Note>

## Databases and the downtime window

The copy is a full dump and restore with row count verification, repeatable, source read-only. There is no incremental sync. To avoid lost writes, pause the Dokploy app, run the final copy, start the app here, then switch DNS. The window is the final copy time plus an app restart. Time your first copy to know it. Stateless apps need no pause. See the [downtime table](/migrate-to-levelrail#what-stays-online-during-the-move).

## Domains and TLS

Dokploy usually runs Traefik on ports 80 and 443. Either point DNS at a new server, or install Levelrail in [coexist mode](/behind-an-existing-proxy). Lower the TTL to 300 seconds a day ahead. Certificates are issued here on first use once DNS points here, so check the first domain before the rest. See [Domains and ingress](/domains-and-ingress).

## Common problems and fixes

| Problem | Fix |
| --- | --- |
| The URL is refused | Allow private networks on the control plane and pass `--allow-private` |
| `401` from Dokploy | Create a new API key and set `APP_IMPORT_SOURCE_TOKEN` |
| Database copy fails | Check host, port and user. Target version must be the same or newer than the source |
| App shows as unsupported | Read the reason. Compose apps and archive uploads are not imported |
| Domain shows `wait` | Lower the TTL and wait out the old value |

## Choose to stay if

Dokploy's Swarm multi-node scheduling and its compose workflow are what you rely on, or you cannot afford a write freeze on your databases. Levelrail does not use Swarm, and its migration path has no recorded run against a live Dokploy. See [Dokploy alternative](/dokploy-alternative) and the [comparison](/comparison).
