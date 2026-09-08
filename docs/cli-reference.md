# CLI reference

`levelrail-cli` is a scriptable client for the control plane's HTTP API. It
covers apps, databases, backups, nodes, IAM, notification channels, feature
flags, and platform migration, roughly 25 top-level command groups with
their own subcommand trees. This page is a reference: every command name
and flag below was captured live from the built binary's own `-h` output
(and cross-checked against `cmd/levelrail-cli/*.go` for nuance `-h` text
doesn't cover), not written from memory.

Every command supports `-h`/`--help` for its own usage, and `<command>
<subcommand> -h` drills into a subcommand's own flags.

## Installing / building

There is no published binary yet. Build it from source the same way the
control plane and agent are built; see the root
[README.md's "Building and running locally"](../README.md#building-and-running-locally)
section for prerequisites (Go 1.26+, Docker). From the repo root:

```
go build -o levelrail-cli ./cmd/levelrail-cli
```

## Global flags and output

Nearly every command accepts the same handful of flags. A few older
subcommands (`apps list`, `apps get`, `databases list`, `domains
basic-auth get`, and others rendered via Go's default `flag.PrintDefaults`)
show them as `-flag` instead of `--flag` in their own `-h` text; both forms
work identically everywhere, since Go's `flag` package treats a single and
double leading dash the same way.

| Flag | Meaning |
| --- | --- |
| `--token` | API token for this call (default: `APP_API_TOKEN` env var, then the credentials file) |
| `--api-url` | control plane base URL (default: `APP_API_URL` env var, then the credentials file, then `http://localhost:8080`) |
| `--profile` | named credentials profile to read (default: `APP_PROFILE` env var, then `"default"`) |
| `--json` | print the result as JSON to stdout and nothing else (shorthand for `--output json`) |
| `--output` | output format: `json`, `table`, or `text` (default `table`) |
| `--query` | a [JMESPath](https://jmespath.org/) expression to filter the result before printing, e.g. `"[?status=='running'].name"` |
| `-h`, `--help` | show the command's usage |

An explicit `--token`/`--api-url` flag always wins over `APP_API_TOKEN`/
`APP_API_URL`, which in turn win over the resolved profile's entry in the
credentials file. `--profile`/`APP_PROFILE` only pick which section of the
credentials file the fallback reads from.

## Authentication and profiles

### `auth login`

Authenticates with a username and password (prompted interactively, with
no echo for the password, when not passed as flags), mints a real API
token from that session, and saves it to `~/.config/levelrail-cli/credentials`.
Every later command's token resolution then finds it automatically.

The username/password path requires the control plane to be reachable over
**https**: the login step's session cookie is `Secure`, so it never rides
back on the follow-up token-mint request against a plain-http target (the
common local-dev default, `http://localhost:8080`). Point `--api-url` at a
real deployment's TLS-terminating ingress, or use `--device` instead, which
works over plain http.

```
# password flow, prompted for username and password, against a real deployment
levelrail-cli auth login --api-url https://cp.example.com

# device code flow: prints a code and URL, waits for approval from the
# dashboard's CLI Access page, works over plain http
levelrail-cli auth login --device --api-url http://localhost:8080

# save under a named profile instead of overwriting "default"
levelrail-cli auth login --profile staging --api-url https://staging.example.com
```

Key flags: `--username`, `--password` (both prompted if omitted),
`--device` (device code flow instead of username/password),
`--client-name` (device flow only, label shown in the approval UI),
`--token-name` (default `levelrail-cli-<timestamp>`), `--abilities`
(comma-separated, default `root`; ignored with `--device`, since the
minted token's abilities always match the approving operator's own
session), `--expires-in-days` (default `0`, never expires).

### `auth whoami`

Calls `GET /api/v1/auth/session` with the resolved bearer token. This
always fails with a real 401 when run the normal way (a persisted API
token from `auth login`): that endpoint is session-cookie-only by
deliberate server-side design (a bearer token has no session of its own to
report on), and this CLI never persists a session cookie, only a bearer
token. There is currently no bearer-token-compatible identity endpoint in
the API this command could call instead. This is not a bug in the command;
it is a real, documented API gap.

### `profile list`

Lists every profile configured in `~/.config/levelrail-cli/credentials`: its
name and API URL, never its token value.

### Credentials file format

`~/.config/levelrail-cli/credentials` is INI-style, one section per profile,
written by `auth login`:

```
# written by "levelrail-cli auth login"
[default]
APP_API_URL=https://cp.example.com
APP_API_TOKEN=<token>

[staging]
APP_API_URL=https://staging.example.com
APP_API_TOKEN=<token>
```

An operator managing more than one control plane saves each under its own
name via `auth login --profile NAME` and switches between them with
`--profile`/`APP_PROFILE`, without re-authenticating or overwriting each
other's credentials. A pre-profile flat credentials file (no `[section]`
headers) is still read as the `default` profile, for backward
compatibility.

### `tokens create` / `tokens list` / `tokens revoke`

Manage API tokens directly. Every subcommand here requires a **live
session** (`--username`/`--password`, prompted if omitted): these routes
are session-only server-side, so this CLI's own persisted bearer token can
never call them.

```
levelrail-cli tokens create --name ci-deploy --abilities deploy
levelrail-cli tokens list
levelrail-cli tokens revoke <id>
```

`--abilities` (create) is a comma-separated list from `read`,
`read:sensitive`, `write`, `write:sensitive`, `deploy`, `root`.
`--expires-in-days` defaults to `0` (never expires).

## Apps

The largest command group: app lifecycle, deploys, organization
(organizations/projects/environments), previews, secrets, git source, and
more, all under `levelrail-cli apps <verb>`.

### Lifecycle and deploys

| Subcommand | Purpose |
| --- | --- |
| `create` | create an app (existing image, git build, `--file`, or `--interactive`) |
| `list` | list apps |
| `get <name>` | show one app |
| `deploy <name>` | deploy an image to an existing app |
| `deploy-compose <name>` | deploy a Docker Compose file as an app |
| `deploy-spec <name>` | fan an `app.yaml`'s `services:` map out into N independent builds under one app |
| `rollback <name>` | redeploy an older image (same endpoint as `deploy`) |
| `restart <name>` | recreate the running container, no image change |
| `stop <name>` / `start <name>` | stop / start an app's running container |
| `delete <name>` | remove an app's desired state |
| `status <name>` | show an app's current reconcile conditions |
| `diagnose <name>` | explain a failed deploy or crashloop, deterministic pattern match, no external model |
| `resource-recommendation <name>` | suggest memory/CPU limits from historical usage, deterministic, never changes anything |
| `network <name>` | show the live traffic path: container port, host port, running |
| `logs <name>` | search an app's stored log entries (historical search, not a live tail) |
| `exec <name> -- <cmd>` | run a command in the app's container, exits with its real exit code |
| `group <name>` | show `<name>`'s sibling services under the same multi-service app |
| `hook-runs <name>` | show the most recent outcome of an app's pre/post-deploy hooks |
| `deploys compare <name>` | diff two deploy attempts, or one against the current live state |
| `promote <name>` | promote `<name>`'s image onto a sibling app in another environment |

`apps exec` requires a literal `--` before the remote command whenever that
command itself takes flags (e.g. `apps exec web -- ls -la`): without it,
this CLI's own flag parser tries to consume the remote flag as its own and
rejects it as unknown. Always safe to include.

`apps deploy` / `apps rollback` / `apps promote` all fail against an app
tagged with a protected environment unless `--confirm` is passed or you
type `yes` at the interactive prompt.

Example workflow: create an app, check it, deploy a new image, roll back.

```
levelrail-cli apps create --name web --image ghcr.io/acme/web:1.0.0 --port 3000

levelrail-cli apps status web

levelrail-cli apps deploy web --image ghcr.io/acme/web:1.1.0

# something's wrong with 1.1.0; go back to the known-good tag
levelrail-cli apps rollback web --image ghcr.io/acme/web:1.0.0

# tail the last 100 stored log lines from the last hour
levelrail-cli apps logs web --since 1h --tail 100
```

`apps create` supports four distinct paths, mutually exclusive:

```
# existing image
levelrail-cli apps create --name web --image ghcr.io/acme/web:1.0.0 --port 3000

# git build (Dockerfile or Railpack)
levelrail-cli apps create --name web --port 3000 \
  --repo https://github.com/acme/web --image-repo ghcr.io/acme/web

# from an app.yaml manifest
levelrail-cli apps create --file app.yaml

# guided wizard
levelrail-cli apps create --interactive
```

### Organization: organizations, projects, environments

| Subcommand | Purpose |
| --- | --- |
| `organizations create/list/get/delete` | manage organizations, which group projects |
| `organizations set-project` / `clear-project` | file a project under an organization / remove it |
| `organizations env-get` / `env-set` | an organization's shared env vars, the base layer projects override |
| `projects create/list/get/delete` | manage projects, which group apps and databases |
| `projects env-get` / `env-set` | a project's shared env vars, between its org's and its environments' |
| `environments create/list/update/delete` | manage a project's environments (staging, production, ...) |
| `environments env-get` / `env-set` | an environment's shared env vars, between its project's and a tagged app's own |
| `set-environment <name> <environment-id>` / `clear-environment <name>` | tag / untag an app with an environment |
| `set-project <name> <project-id>` / `clear-project <name>` | move an app into / out of a project |

`environments create --protected` marks an environment so any
`deploy`/`rollback`/`promote` targeting a tagged app requires `--confirm`.
Env var precedence, narrowest wins: app's own env overrides environment's,
which overrides project's, which overrides organization's.

### Previews, secrets, git source, webhooks

| Subcommand | Purpose |
| --- | --- |
| `previews list <app>` | list active pull-request preview environments |
| `previews enable` / `disable <app>` | opt an app into / out of preview environments |
| `previews teardown <app> <pr-number>` | tear down one PR's preview right now |
| `previews sweep` | tear down every stale preview now, across all apps (manual trigger for the automatic TTL fallback) |
| `previews pr-status enable/disable <app>` | opt into a GitHub PR comment/commit status per preview deploy |
| `secrets list <app>` | list an app's secret keys and locked state, never a value |
| `secrets set <app> <key> <value>` | set or rotate one secret's value |
| `secrets lock <app> <key>` | toggle a secret's accidental-overwrite guard, reversible either way |
| `git-source get/set/delete <app>` | connect, edit, or disconnect a repo for auto-deploy-on-push |
| `webhook-deliveries list <app>` | inspect recent inbound git webhook requests, verified or not |
| `webhook-deliveries replay <app> <id>` | re-run a stored delivery's payload, can trigger a real build and deploy |

`previews enable`/`disable`/`pr-status` require a connected git source
(`apps git-source set`); the CLI does not yet expose the multi-service
fan-out (`app.yaml`'s `services:` map) for `git-source set` itself, that's
dashboard-only, use `apps deploy-spec` instead.

### Scheduled tasks, alerts, log drain

| Subcommand | Purpose |
| --- | --- |
| `scheduled-tasks create <app> --schedule CRON -- <cmd>` | run an arbitrary command inside the app's container on a cron schedule |
| `scheduled-tasks list/get/update/delete <app>` | manage scheduled tasks |
| `scheduled-tasks run <app> <id>` | trigger an immediate run |
| `alerts list/delete <app>` | manage an app's alert rules |
| `alerts create <app> --kind KIND` | create a `threshold`, `crashloop`, `cert_expiry`, `patch_status`, `scheduled_task_failure`, `node_disk_space`, `node_resource_usage`, or `domain_health` rule |
| `log-drain get/set/clear <app>` | configure forwarding an app's logs to an external HTTP or syslog sink, additive to the built-in log store |

`cert_expiry`, `patch_status`, `node_disk_space`, and `node_resource_usage`
alert rules are platform-wide (they watch every certificate/node, not just
`<app>`'s own); `<app>` only decides where the rule shows up in `alerts
list`.

## Databases

Managed databases: Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB,
Dragonfly, ClickHouse.

| Subcommand | Purpose |
| --- | --- |
| `create` | create a managed database (`--interactive` for a guided wizard) |
| `list` | list databases |
| `get <name>` | show one database |
| `delete <name>` | remove a database's desired state |
| `resource-recommendation <name>` | suggest memory/CPU limits from historical usage |
| `set-project` / `clear-project <name>` | move a database into / out of a project |

```
levelrail-cli databases create --name main --engine postgres --version "16"

levelrail-cli databases get main

levelrail-cli databases resource-recommendation main
```

Creating a Postgres database always succeeds at the API level; the
reconciler refuses to actually start it until secrets exist for it, and
reports that as a real condition visible in `databases get main`.

## Backups (databases) and app volume backups

`backups` covers managed database backups; `app-volume-backups` covers
named volumes declared in an app's `app.yaml` (see `apps get <app>` to
list its volumes). Both share the exact same subcommand shape.

| Subcommand | Purpose |
| --- | --- |
| `list <database>` (or `<app> <volume>`) | list backup attempt history |
| `trigger --target ID` | start a manual backup (asynchronous, returns once the attempt is recorded) |
| `restore --backup ID --confirm NAME` | overwrite live data in place from a backup, destructive, requires typing the exact name to confirm |
| `restore-as-new --backup ID` | restore into a brand-new database/volume, non-destructive |
| `verify --backup ID` | re-download a backup and confirm it's intact, no live restore |
| `verifications --backup ID` | list past verification attempts |
| `schedule set --target ID --cron EXPR` | configure a recurring backup |
| `schedule clear` | remove a recurring backup |

```
levelrail-cli backups trigger main --target s3-primary

levelrail-cli backups list main

# destructive: overwrites main's live data
levelrail-cli backups restore main --backup <backup-id> --confirm main

# safe: clones into a brand-new database, main is untouched
levelrail-cli backups restore-as-new main --backup <backup-id> --new-name main-staging-copy

levelrail-cli backups schedule set main --target s3-primary --cron "0 3 * * *" --retain 7
```

For app volumes, the restore confirmation is `<app>/<volume>` instead of a
bare database name: `app-volume-backups restore web data --backup <id>
--confirm web/data`.

## Domains

| Subcommand | Purpose |
| --- | --- |
| `list` | list every app's domains in one call |
| `cloudflare-dns get/set/clear` | configure the Cloudflare API token ACME uses for wildcard-domain DNS-01 challenges |
| `basic-auth get/set/clear <app> <domain>` | protect one of an app's domains with HTTP Basic Auth |
| `maintenance get/set/clear <app> <domain>` | serve a fixed "down for maintenance" page instead of proxying to the container |
| `tls-cert get/set/clear <app> <domain>` | upload or clear a domain's own (BYO) TLS certificate |

`domains cloudflare-dns` is a distinct credential from `cloudflare-tunnel`
(below): one configures the DNS-01 ACME challenge, the other exposes the
control plane through Cloudflare Tunnel. `<domain>` in `basic-auth`,
`maintenance`, and `tls-cert` must already be one of the app's configured
domains.

## Nodes

| Subcommand | Purpose |
| --- | --- |
| `list` / `get <id>` | list nodes / show one node |
| `delete <id>` | delete a node, fails (409) while it still has placements, drain first |
| `join-token` | mint a one-time enrollment token for a new node, shown once |
| `cordon <id>` / `uncordon <id>` | mark unschedulable / schedulable again, without evacuating anything |
| `drain <id> [--target ID]` | move every service and database off a node (default target: the local node) |
| `workloads <id> --accepts-app --accepts-build` | set a node's accepted workload kinds, a full replace of both flags |
| `health <id>` | show a node's current reconcile conditions |
| `patch-status <id>` | show a node's latest available-OS-updates reading |

## IAM, users, and roles

| Subcommand | Purpose |
| --- | --- |
| `users list` | list every user |
| `users create --email --password (--role \| --abilities)` | create a local-password user |
| `users set-abilities <id> (--role \| --abilities)` | replace a user's abilities wholesale, refused for the caller's own user |
| `users delete <id>` | remove a user, refused for the caller's own user and the last remaining user |
| `users roles` | list the curated role presets `--role` accepts (`admin`, `operator`, `viewer`) |
| `iam policies create --name --document` | create a resource-scoped Allow/Deny policy, additive on top of `--abilities` |
| `iam policies list/get/update/delete` | manage policies |
| `iam policies attach/detach --principal-type --principal-id` | attach/detach a policy to a user or token |
| `iam policies attachments <id>` | list a policy's attached principals |

`--document` accepts either an inline JSON string or `file://path/to/policy.json`:

```
levelrail-cli iam policies create --name deploy-only-web \
  --document '{"Statement":[{"Effect":"Allow","Action":["deploy"],"Resource":["app:web"]}]}'
```

## Notification channels

```
levelrail-cli channels create --name oncall-slack --kind slack --notify-url https://hooks.slack.com/services/...
levelrail-cli channels test <id>
levelrail-cli channels deliveries <id>
```

| Subcommand | Purpose |
| --- | --- |
| `list` | list connected notification channels |
| `create --name --kind` | connect a new channel (`--kind`: `generic`, `slack`, `discord`, `telegram`, `email`, `pushover`, `pagerduty`, `teams`) |
| `delete <id>` | disconnect a channel (apps attached to it keep their own notify-target row, with the channel reference cleared) |
| `test <id>` | send a real test message to a connected channel |
| `deliveries <id>` | list a channel's recorded send history (deploy outcomes, alert rules, test sends) |

`pushover` uses `--pushover-user-key`/`--pushover-api-token` instead of
`--notify-url`; `pagerduty` uses `--pagerduty-routing-key`.

## Feature flags

A boolean, plus an optional gradual rollout percentage, an app's own
running code reads live via `GET /api/v1/flags/evaluate/{key}`. Changes
take effect immediately, no redeploy or restart needed, since a flag's
value is never baked into a container.

```
levelrail-cli flags create web --key new-checkout --name "New checkout flow" --rollout 25
levelrail-cli flags list web
levelrail-cli flags set web <id> --name "New checkout flow" --rollout 100
```

`--key` is globally unique across the whole control plane and cannot be
changed after create.

## Backup targets and registry credentials

Both are connection profiles referenced by id elsewhere (`backups trigger
--target`, an app's image pull), never returning their secret material
back once stored.

```
levelrail-cli backup-targets create --name primary --provider r2 \
  --endpoint https://<account>.r2.cloudflarestorage.com \
  --bucket backups --access-key-id <id> --secret-access-key <key>

levelrail-cli backup-targets test <id>

levelrail-cli registry-credentials create --name ghcr \
  --registry-host ghcr.io --username <user> --password <token>
```

Valid `backup-targets --provider` values: `aws`, `r2`, `custom`.
`--endpoint` is required for `r2` and `custom` (`aws` resolves its own
default endpoint per region). Both groups' `update` subcommand rotates the
stored credential only when the credential flags are passed again; omit
them to leave the existing one unchanged. Both `test` subcommands probe
the live connection without uploading, deleting, or pulling anything.

## Cloudflare Tunnel

```
levelrail-cli cloudflare-tunnel set --cf-tunnel-token <token>
levelrail-cli cloudflare-tunnel get
levelrail-cli cloudflare-tunnel disconnect
```

Runs the `cloudflared` container connected to a tunnel token generated in
your own Cloudflare Zero Trust dashboard, exposing the control plane
without opening an inbound port. Distinct credential from `domains
cloudflare-dns`.

## Observability and platform status

| Subcommand | Purpose |
| --- | --- |
| `status` | control plane status, including local Docker daemon reachability |
| `version` | running control plane version, and whether a newer release is published |
| `doctor` | local preflight health check: Docker, disk space/write access, port 80/443 availability, database reachability. Exit code 1 if any check fails |
| `containers` | every container on this node, managed by `levelrail-cli` or not (read-only; use `apps stop/start/restart` to actually manage one) |
| `audit-log` | every recorded write/deploy/root-tier request, newest first; requires an admin/root-scoped token |
| `audit-purge` | delete audit log entries past the retention window right now, instead of waiting for the automatic sweep |

`audit-log` supports `--format csv --output-file FILE` for exporting, plus
`--path`, `--method`, and `--client-kind` (`cli`, `dashboard`, `mcp`, or
`api`) filters.

## Secrets: master key rotation

```
levelrail-cli secrets rotate-master-key --new-key-file ./new.key
```

Re-wraps every stored per-app data encryption key from the currently
active master key to a new one, in one atomic step, live, while the
control plane keeps serving. The new key is read from a file (or `-` for
stdin) so it never appears as a bare command-line argument. Read
[docs/master-key-rotation.md](master-key-rotation.md) before running this
in production.

## Migrating from Coolify, Dokploy, or CapRover

```
levelrail-cli migrate coolify --url https://coolify.example.com --token <token>
levelrail-cli migrate dokploy --url https://dokploy.example.com --token <api-key>
levelrail-cli migrate caprover --url https://captain.example.com --token <login-password>
```

One-way migration from a live instance. Default mode writes `app.yaml`
files plus a migration report into `--out-dir` (default `./migrated`);
`--apply` creates apps directly on a target Levelrail instance instead.
`--include-secret-values` pulls real env var values across (for Coolify,
requires a token with the `read:sensitive` ability).

Source-specific notes, each a real gap flagged by the command itself, not
silently dropped:

- **Coolify/Dokploy**: git repo/branch identity isn't persisted Levelrail
  state today, so the report prints the exact `apps create --repo ...
  --ref ...` follow-up command for each migrated app that had one.
- **Dokploy**: source types with no Levelrail equivalent (`docker`,
  `drop`) and build types (`nixpacks`, `heroku_buildpacks`,
  `paketo_buildpacks`) are reported for manual review, not migrated.
- **CapRover**: its `captain-definition` build config isn't exposed by its
  API, so every migrated app's build type is assumed `dockerfile` and
  flagged for manual review. CapRover also exposes no git repo/branch
  identity at all, so no follow-up `apps create --repo ...` command is
  printed for it.

See [docs/migrating-from-coolify-and-dokploy.md](migrating-from-coolify-and-dokploy.md)
for a full walkthrough.

## Shell completion

```
levelrail-cli completion bash    # print a bash completion script
levelrail-cli completion zsh     # print a zsh completion script
levelrail-cli completion fish    # print a fish completion script
```

Completes command and subcommand names for the full command tree, plus the
global flags. It does not complete flag values or positional arguments
like app names.

```
# bash
source <(levelrail-cli completion bash)
# or persist it:
levelrail-cli completion bash | sudo tee /etc/bash_completion.d/levelrail-cli > /dev/null

# zsh
source <(levelrail-cli completion zsh)
# or persist it:
levelrail-cli completion zsh > "${fpath[1]}/_levelrail-cli"

# fish
levelrail-cli completion fish | source
# or persist it:
levelrail-cli completion fish > ~/.config/fish/completions/levelrail-cli.fish
```

Note: `completion bash -h`, `completion zsh -h`, and `completion fish -h`
do not print help text the way every other command's `-h` does; they
print the full completion script instead, silently ignoring the trailing
`-h`. Only bare `completion -h` (with no shell argument) shows the usage
above.
