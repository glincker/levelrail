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
| Redirect rules | A rule whose regex starts at the app's first domain and whose replacement is an absolute http or https URL becomes a forwarder rule on that domain on `--apply`. Anything else is listed as a review item |
| Basic auth | Only the fact that it was on. `--apply` gates each domain with a random placeholder password that is never shown; set the real one with `domains basic-auth set`. The source password is never read |
| Pre and post deploy commands | Carried over as deploy hooks when the command is one line and holds no inline secret or env value. A static site has no container, so they are reported instead |
| Extra published ports | Reported as a review item, they are not routed |
| Health check | Only when it is a curl or wget style HTTP probe |
| Databases | PostgreSQL, MySQL, MariaDB, MongoDB and Redis, created empty with the major version from the image tag |

## What needs a manual step

- **Git webhooks**: pass `--create-webhooks` with `--apply` to register one push webhook per git-sourced app after it is created. It is off by default because it writes to your git host. A hook is only created when the repository URL is https and its scheme, host and port match an authorized git connection (GitHub App, GitLab, Gitea or Bitbucket), and it goes through the existing `apps git-source register-webhook` route with your own token. The control plane generates the webhook secret, which is never printed or written to the report. A re-run does not add a second hook. Every app that is skipped gets the reason, and a failure on one app does not stop the others. Without `--apply` the report lists what would be created and writes nothing to the git host. `APP_MIGRATE_WEBHOOK_LIMIT` (default 200) caps hooks per run and `APP_MIGRATE_REPO_URL_MAX_LEN` (default 2048) caps the accepted repository URL length.
- **Backup destinations and schedules**: each Dokploy S3 destination becomes a backup target with the same name (lowercased), endpoint, region and bucket, and each database backup schedule becomes the backup schedule of the database with the same name on Levelrail, keeping its cron and its keep-latest count as `--retain`. The keys are used only with `--apply` and `--include-secret-values`: they go straight into the normal backup target create call, which stores them encrypted, and are never printed, written to the report or journaled. Without both flags, or when Dokploy returns masked keys, the target is reported `needs_secret` with the `backup-targets create` command to run. A target or schedule whose name already exists on Levelrail is skipped with a warning, never overwritten. Imported schedules start paused; pass `--enable-backup-schedules` to keep the ones that were on in Dokploy running. Bucket names must follow the S3 naming rule; anything else is reported for review. Endpoints must be https and are checked against the import network policy (`APP_IMPORT_ALLOW_PRIVATE_NETWORKS`, `APP_IMPORT_ALLOW_LOOPBACK`); the control plane connects to the endpoint from its own network, so confirm each one before enabling schedules. Levelrail keeps one schedule per database, so a second Dokploy schedule on the same database is reported, not imported. A schedule whose database is not on Levelrail yet is reported with the `backups schedule set` command to run after the database moves. `APP_MIGRATE_BACKUP_LIMIT` (default 200) caps destinations plus schedules per run.
- **Notification providers**: each Dokploy provider becomes an alert channel with the same name (lowercased). Slack, Discord, Telegram, email, Gotify, ntfy, Lark and Pushover map onto the channel kind of the same name and a custom webhook becomes a `generic` channel. Webhook URLs, bot tokens and app tokens are read from Dokploy only on `--apply` with `--include-secret-values`, in a separate call, and go straight to the normal channel create call; they never appear in the report, the plan or the journal. Without them the channel is reported `needs_secret` with a `channels create` command that holds placeholders only. Every URL must be https, carry no user info and resolve outside internal ranges (`APP_IMPORT_ALLOW_PRIVATE_NETWORKS` relaxes that), and the channel create endpoint applies its own checks on top. Email channels send through this server's SMTP settings; the Dokploy SMTP server and password are not read, and only the first recipient is imported. Channels are created disabled so a migration cannot start sending alerts; pass `--enable-imported-channels` to create them enabled. An enabled channel receives broadcast notices (key rotation reminders, maintenance and update notices) right away, with no routing needed. If the server returns a channel enabled although disabled was requested, the import deletes it and reports the step failed. A name that already exists is skipped with a warning, never overwritten. A custom webhook with extra headers, a blank or symbols-only name and kinds with no match here (listed as unsupported with the reason) need a manual step. The per-run cap is `APP_MIGRATE_BACKUP_LIMIT`, shared with backup imports.
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

## Resume or undo an --apply run {#apply-journal}

Every `migrate dokploy --apply` run writes a journal: one entry per app with each step (created, git source, node, ingress, previews, secrets, files, webhook) and its outcome. The journal holds names, step states and a fingerprint of each created app. It never holds env values, secrets, tokens, passwords or connection strings. It is written with mode `0600`, atomically, and a lock file stops two runs from using it at once. The default path is under `~/.config/<cli>/migrate-journals/`; pick one with `--journal FILE`. The path is printed at the end of the run.

If the run stopped half way, continue it with the same flags:

```
levelrail-cli migrate dokploy resume --journal FILE --url URL --token TOKEN [the same flags as the --apply run]
```

Resume skips steps that are done and retries failed ones. It never creates an app that already exists and was not created by this run, and it leaves an app alone if it changed on the target since the run.

To roll the run back:

```
levelrail-cli migrate dokploy undo --journal FILE           # prints the plan, removes nothing
levelrail-cli migrate dokploy undo --journal FILE --yes     # removes what the plan marks remove
```

Imported alert channels are journaled by name, channel id and a fingerprint of kind, name, id, creation and update time, never with a token or URL. Undo deletes a channel only when that id still lists with the same fingerprint; when this token sees no channels, or not that id, undo reports that it cannot tell and removes nothing.

Imported backup targets and schedules are journaled by name, id, shape fingerprint (provider, endpoint, bucket, region) or cron, and state, never with keys. Undo clears the schedules and deletes the targets the run created only when they still read back by that id with the same shape or cron, and never touches ones that existed before. A target your token cannot see is reported as unknown, never as gone, and resume does not create it again.

Undo removes only the apps the journal records as created by that run, newest first, with your own token and the normal delete route, so protected environments and delete confirmations still apply. Apps that existed before the run are never touched. An app deployed since the run is listed and skipped unless you pass `--force`. An app whose fingerprint is missing or no longer matches (it changed since the run, or the run stopped before recording it) is always skipped unless you name it with `--force-app NAME`; `--force` never overrides that. Undo checks each app again right before it deletes it. Apps have no immutable id in the API, so the fingerprint is the app's shape (image, domains, port, node, volumes and similar, never env values): if someone deletes an app and re-creates one with the same name and shape, undo cannot tell them apart, so read the plan before passing `--yes`. An app holding volume data is skipped unless you pass `--include-data` and type the number of such apps to confirm; its volumes are then deleted with it. Undo refuses a journal that is a symlink or sits in a symlinked folder, readable by others, over `APP_MIGRATE_JOURNAL_MAX_BYTES` (default 4 MiB), written for another control plane, or with a name that is not a valid app name.

## Choose to stay if

Dokploy's Swarm multi-node scheduling and its compose workflow are what you rely on, or you cannot afford a write freeze on your databases. Levelrail does not use Swarm, and its migration path has no recorded run against a live Dokploy. See [Dokploy alternative](/dokploy-alternative) and the [comparison](/comparison).

## Compose stacks

`migrate dokploy` imports each raw compose stack as a [compose app](/compose-apps). The compose file is checked with the same validation a manual compose deploy runs, so a stack with `privileged`, host networking, host `pid` or `ipc`, `cap_add`, `devices` or a Docker socket mount stays blocked with that reason. A ready stack is staged as `compose.yaml` under `--stage-dir` (default `./migrated/compose`), with its domains and named volumes in the report. The stack env is split into secret and plain values. Without `--include-secret-values` only the names are reported; with it, all values go into `stack.env` (mode 0600), and the printed deploy command ends with `apps secrets set <app> --env-file stack.env`. A value spanning lines is never written and is reported instead. Files larger than `APP_IMPORT_COMPOSE_MAX_BYTES` (default 256 KiB) are refused. Dokploy file mounts on applications become app files at their mount path; unsafe paths are refused.
