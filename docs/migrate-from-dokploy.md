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

## What moves

| Item | Result |
| --- | --- |
| Applications | Image apps (`docker` source), git apps (GitHub, Bitbucket, custom git URL, branch), build method (`dockerfile`, `static`, nixpacks and buildpacks become Railpack) |
| Port | The port of the first domain |
| Environment variables | The `env` text block parsed. Secret-looking names go through the encrypted secrets store |
| Domains | The domain list |
| Volumes | Volume and bind mounts |
| Replicas, memory and CPU limits | Imported |
| Preview deployments | The preview limit becomes the app's preview policy maximum on `--apply`. The wildcard host, auto deploy off and clean cache are reported as review items |
| Health check | Only when it is a curl or wget style HTTP probe |
| Databases | PostgreSQL, MySQL, MariaDB, MongoDB and Redis, created empty with the major version from the image tag |

## What needs a manual step

- **Git webhooks**: pass `--create-webhooks` with `--apply` to register one push webhook per git-sourced app after it is created. It is off by default because it writes to your git host. A hook is only created when the repository URL is https and its scheme, host and port match an authorized git connection (GitHub App, GitLab, Gitea or Bitbucket), and it goes through the existing `apps git-source register-webhook` route with your own token. The control plane generates the webhook secret, which is never printed or written to the report. A re-run does not add a second hook. Every app that is skipped gets the reason, and a failure on one app does not stop the others. Without `--apply` the report lists what would be created and writes nothing to the git host. `APP_MIGRATE_WEBHOOK_LIMIT` (default 200) caps hooks per run and `APP_MIGRATE_REPO_URL_MAX_LEN` (default 2048) caps the accepted repository URL length.
- Multi-server installs: pass `--node-map source-server=node` (the Dokploy server name or id) to put each app on the named node. An unmapped server keeps its apps on the control plane node with a finding, and an unknown or ambiguous node name stops the run before anything is created. Placement needs a token with the root ability, checked first.

- Database contents and volume contents (below).
- Git apps do not build by themselves: connect the repository and run a build.
- Health checks that are shell commands, extra ports, pre and post deploy commands.
- Dokploy scheduled jobs are not read. Recreate them as scheduled tasks.

## What does not move

- Compose stacks whose file lives in a git repository. They are reported blocked; deploy them from git with a compose build.
- Apps uploaded as an archive (`drop`).
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
| App shows as unsupported | Read the reason. Archive uploads and blocked compose stacks are not imported |
| Domain shows `wait` | Lower the TTL and wait out the old value |

## Verify and roll back

Run `levelrail-cli migrate cutover --verify` after you switch DNS. It checks the certificate, the health path and DNS.

The copy only reads the old server, so nothing there is changed. To roll back, restart the old app if you paused it and point DNS back at the old server.

## Choose to stay if

Dokploy's Swarm multi-node scheduling and its compose workflow are what you rely on, or you cannot afford a write freeze on your databases. Levelrail does not use Swarm, and its migration path has no recorded run against a live Dokploy. See [Dokploy alternative](/dokploy-alternative) and the [comparison](/comparison).

## Compose stacks

`migrate dokploy` imports each raw compose stack as a [compose app](/compose-apps). The compose file is checked with the same validation a manual compose deploy runs, so a stack with `privileged`, host networking, host `pid` or `ipc`, `cap_add`, `devices` or a Docker socket mount stays blocked with that reason. A ready stack is staged as `compose.yaml` under `--stage-dir` (default `./migrated/compose`), with its domains and named volumes in the report. The stack env is split into secret and plain values. Without `--include-secret-values` only the names are reported; with it, all values go into `stack.env` (mode 0600), and the printed deploy command ends with `apps secrets set <app> --env-file stack.env`. A value spanning lines is never written and is reported instead. Files larger than `APP_IMPORT_COMPOSE_MAX_BYTES` (default 256 KiB) are refused. Dokploy file mounts on applications become app files at their mount path; unsafe paths are refused.
