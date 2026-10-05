---
description: Complete reference of all Levelrail CLI commands, organized by group with examples and typical use cases.
---

# Levelrail CLI Reference

`levelrail-cli` is a scriptable client for the control plane's HTTP API. Everything it does goes through `/api/v1`, with the same tokens and permissions as the dashboard, so it needs no SSH key and no open port on your servers. This page lists every command group with its flags and typical use. Run `levelrail-cli` with no arguments for the command list, or `levelrail-cli <command> <subcommand> -h` for any command's own flags.

Commands are shown as `levelrail-cli ...`. If you rename the binary, the command name follows it (the CLI reads its name from `os.Args[0]`). The server is a separate binary, `levelrail`, which also has a few maintenance commands such as `setup-token`, `restore`, and `healthcheck`. Those are covered in [Installing](installing.md) and [Disaster recovery](/disaster-recovery).

## Install and sign in

```bash
curl -fsSL https://levelrail.com/install-cli.sh | sh
export APP_API_URL=https://console.example.com
levelrail-cli auth login --device
levelrail-cli auth whoami
```

`--device` prints a short code that you approve in the dashboard, the same model as `gh auth login`. Credentials are saved to `~/.config/levelrail-cli/credentials`. For CI, create an API token under **Settings, API tokens** and pass it with `--token` or `APP_API_TOKEN`.

Each command resolves its target in this order, and an explicit flag always wins:

| Setting | Flag | Environment variable | Default |
| --- | --- | --- | --- |
| API token | `--token` | `APP_API_TOKEN` | the saved profile |
| Control plane URL | `--api-url` | `APP_API_URL` | `http://localhost:8080` |
| Profile | `--profile` | `APP_PROFILE` | `default` |

Manage more than one control plane with named profiles: `levelrail-cli auth login --profile staging`, then `--profile staging` on any command. `levelrail-cli profile list` shows them.

## See also

- [Getting Started](getting-started.md) - First steps with Levelrail
- [Deploying and managing apps](deploying-apps.md) - The app lifecycle, command by command
- [Feature Catalog](feature-catalog.md) - Complete feature overview
- [App Spec Reference](app-spec-reference.md) - YAML configuration syntax

<InlineToc default-open />

## Scripting: `--json` and exit codes

Every command that returns data or a result supports `--json` (shorthand for `--output json`), and `--query` takes a JMESPath expression. With `--json`, stdout carries only the JSON result. The exceptions are `completion bash|zsh|fish` (a shell script) and `control-plane-backups help-dr` (a static runbook). A test walks the command tree and fails when a new command has no `--json` and is not on that exempt list.

On failure, `--json` also prints an error object to stdout (the message still goes to stderr):

```json
{"error": "server returned 404: app not found", "code": "not_found", "exit_code": 4, "http_status": 404, "hint": "check the resource name with the matching list command"}
```

`error` is the original field. `code` is one of `validation`, `network`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `rate_limited`, `invalid_request`, `server_error`, `api_error`. `http_status`, `retry_after` (rate limits) and `hint` appear when they apply.

Exit codes are stable and shared by every command:

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Usage error (unknown command, missing argument, bad flag); also a failed check for `control-plane-backups verify` and a critical item for `attention` |
| 2 | Validation error: well-formed flags, but the request they describe is invalid |
| 3 | Network error: the control plane could not be reached |
| 4 | API error: the control plane replied with a non-2xx status (use `code` and `http_status` in the JSON error to tell auth, not found and conflict apart) |
| 5 | Deploy failed: `apps wait` reached a failed deploy |
| 6 | Deploy timeout: `apps wait` gave up before the deploy converged |

Codes 3 and 4 are broad on purpose so existing scripts keep working; the JSON error object carries the finer distinction.

## Scripting: `--debug`

`--debug` works anywhere on the command line (before or after the subcommand) and traces every outgoing request this invocation makes to stderr: method, URL, and the response status and timing, one line per call. It never prints request/response headers or bodies, so `Authorization` and any token value never appear, even in debug mode; a token that somehow ended up in a URL's query string is also redacted. stdout is untouched, so `--debug` composes with `--json`/`--query` for scripting.

```
levelrail-cli apps list --debug --api-url http://10.0.0.5:8080
```
```
DEBUG: GET http://10.0.0.5:8080/api/v1/apps -> 200 OK (42ms)
```

## Top-level convenience aliases

```
levelrail-cli deploy <name> --image IMAGE [flags]
```
Alias for `apps deploy`; deploy an image to an existing app.

```
levelrail-cli rollback <name> --image IMAGE [flags]
```
Alias for `apps rollback`; redeploy an older image.

## Apps

`apps list` prints every app the caller can read as a bare JSON array, unchanged from before. On a fleet large enough that matters, add `--max-items N` to cap the page size; the response becomes `{"items": [...], "next_token": "...", "total_count": N}` instead, and an empty `next_token` means there is no further page. Pass that `next_token` back in as `--starting-token` to fetch the next page (requires `--max-items`, since the server only pages a result when a limit is set):

```
levelrail-cli apps list --max-items 50 --json
levelrail-cli apps list --max-items 50 --starting-token 50 --json
```


```
levelrail-cli apps alerts create <app> --name NAME --kind threshold --metric METRIC --comparator OP --threshold N [flags]
```

```
levelrail-cli apps alerts delete <app> <id> [flags]
```

```
levelrail-cli apps alerts list <app> [flags]
```

```
levelrail-cli apps alerts update <app> <id> --name NAME --kind threshold --metric METRIC --comparator OP --threshold N [flags]
```

```
levelrail-cli apps auto-rollback enable <app-name> [flags]
```

```
levelrail-cli apps auto-rollback disable <app-name> [flags]
```

```
levelrail-cli apps auto-rollback status <app-name> [flags]
```

```
levelrail-cli apps auto-rollback-slo-burn set <app-name> off|auto|dry_run|pause_for_human [flags]
```

```
levelrail-cli apps auto-rollback-slo-burn status <app-name> [flags]
```

```
levelrail-cli apps health get <name> [flags]
```

```
levelrail-cli apps health set <name> --probe readiness|liveness (--path PATH | --exec CMD) [--scheme https] [--host HOST] [--tls-skip-verify] [--follow-redirects true|false] [--expected-status 200-399] [--interval 5s] [--timeout 2s] [--failures 3] [--ready-timeout 90s] [flags]
```

```
levelrail-cli apps health clear <name> [--probe readiness|liveness] [flags]
```

```
levelrail-cli apps health-score <name> [flags]
```
synthesized pass/warn/fail readiness verdict across deploy health, security, resilience, and observability

```
levelrail-cli apps builds trigger <name> --repo URL --ref REF [flags]
```
build an image from a git source and deploy it to an existing app

```
levelrail-cli apps clear-environment <name> [flags]
```

```
levelrail-cli apps clear-project <name> [flags]
```

```
levelrail-cli apps clone <name> <new-name> [flags]
```

```
levelrail-cli apps save-as-template <name> [--template-name NAME] [--description TEXT] [flags]
```
derives a compose.yaml from `<name>`'s current desired state and saves it as a reusable template; no secret, database, or vault-backed env value is ever captured, only the key name

```
levelrail-cli apps connect <app> <database> [--field FIELD] [--env-var NAME] [flags]
```
connect `<app>` to a managed database, injecting its resolved connection value as an env var; unlike `apps database`, an app can have any number of these

```
levelrail-cli apps connections list <app> [flags]
```
list `<app>`'s current database connections, including whether each resolves to a mesh DNS name (cross-node-capable) or a container name

```
levelrail-cli apps connections suggest <app> [flags]
```
list managed databases `<app>` could connect to, marking which are already connected

```
levelrail-cli apps create --name NAME --image IMAGE --port PORT [flags]
```

```
levelrail-cli apps create [flags]
```
create an app (existing image, git build, --file, or --interactive)

```
levelrail-cli apps database set <name> --database-name NAME [flags]
```
attach an already-created managed database to `<name>` as its connection-env-var source

```
levelrail-cli apps database clear <name> [flags]
```
detach the database `<name>` currently resolves its connection env var from

```
levelrail-cli apps delete <name> [flags]
```

```
levelrail-cli apps disconnect <app> <env-var> [flags]
```
remove one database connection from `<app>` by its env var name

```
levelrail-cli apps deploy <name> --image IMAGE [flags]
```

```
levelrail-cli apps wait <name> [flags]
```
poll until a deploy attempt actually converges, exit accordingly (a CI gate for "apps deploy"). On success it says what happened: `rolled out`, `already up to date` (the deploy changed nothing) or `restarted`; `--json` carries the same as `outcome`

```
levelrail-cli apps timeline <name> [--limit N] [flags]
```
what happened to an app, newest first: deploys, rollbacks, restarts, env, secret and config changes (key names only, never values), scaling, stop and start

```
levelrail-cli apps apply <name> [flags]
```
restart an app so saved env, secret and config changes reach the running container; does nothing when nothing is pending

```
levelrail-cli apps domains list <name> [flags]
levelrail-cli apps domains add <name> <domain>... [flags]
levelrail-cli apps domains remove <name> <domain>... [flags]
```
show or change an app's domains; a domain already used by another app is refused and nothing is changed

```
levelrail-cli apps deploy-compose <name> --file compose.yaml [flags]
```

```
levelrail-cli apps validate --file <app.yaml|compose.yaml> [flags]
```
parse and validate an app.yaml or a Docker Compose file locally, no API call and no deploy; prints the detected format, service count, and every non-blocking `notices` entry a real deploy would also surface

```
levelrail-cli apps deploy-notify-targets create <app> --channel-id ID [flags]
```

```
levelrail-cli apps deploy-notify-targets delete <app> <id> [flags]
```

```
levelrail-cli apps deploy-notify-targets list <app> [flags]
```

```
levelrail-cli apps deploy-spec <name> --file app.yaml --repo-url <url> --ref <ref> [flags]
```

```
levelrail-cli apps deploys list <name> [flags]
```
real, row-per-attempt deploy history, newest first

```
levelrail-cli apps deploys compare <name> --from ID [--to ID] [flags]
```
diff two deploy attempts, or one against the current live state

```
levelrail-cli apps deploys logs <name> <deploy-id> [flags]
```
one deploy attempt's full build/log output, printed to stdout (redirect to a file to save it)

```
levelrail-cli apps deploys wait <name> [deploy-id] [--timeout 10m] [--poll-interval 2s] [flags]
```
blocks until one deploy is healthy, failed, canceled, superseded or blocked and prints the result with its failure; exits 0 healthy, 7 not healthy, 6 timeout

```
levelrail-cli apps deploys show <name> [deploy-id] [flags]
```
one deploy attempt (the newest by default) with its structured failure: code, cause, failing step, redacted log excerpt, suggested fix, docs link and retryable, see [Deploy failures](deploy-failures.md)

```
levelrail-cli apps deploys failed [--since 24h] [flags]
```
every app's latest failed deploy in the window (default set by the server), with the image of its newest good deploy as a rollback target

```
levelrail-cli deployments list [--status a,b] [--app NAME] [--branch B] [--trigger T] [--environment E] [--since 24h] [--until T] [--q TEXT] [--live] [--pr N] [--limit N] [--cursor C] [flags]
levelrail-cli deployments summary [--window 24h] [flags]
levelrail-cli deployments watch [flags]
```
deploys across every app you can read: a filterable newest-first list (with `--cursor` paging), a status and duration summary, and a live event stream (`--json` prints one object per event)

```
levelrail-cli apps deploys steps <name> <deploy-id> [flags]
```
stream one deploy attempt's pipeline steps (detecting, building, pushing, deploying) until it ends; exits non-zero if a step failed. An already-finished attempt replays only a short two-point summary

```
levelrail-cli deploy-approvals list [--status pending|all|approved|rejected|expired] [--service NAME] [flags]
```
list deploy approvals (status defaults to pending)

```
levelrail-cli deploy-approvals get <id> [flags]
```

```
levelrail-cli deploy-approvals approve <id> [flags]
```
approve a pending deploy; the gated deploy/promote runs now

```
levelrail-cli deploy-approvals reject <id> [--reason TEXT] [flags]
```
reject a pending deploy; the app's desired state is left untouched

```
levelrail-cli apps environments clone <id> --new-name NAME [--app-rename SOURCE=NEWNAME ...] [--domain SOURCE=D1,D2 ...] [--copy-secret-values] [flags]
```
clone a whole environment's app set plus config into a new environment

```
levelrail-cli apps environments clone-preview <id> --new-name NAME [flags]
```
preview what cloning an environment would create, without applying it

```
levelrail-cli apps environments create <project-id> --name NAME [--protected] [flags]
```
create an environment under a project

```
levelrail-cli apps environments delete <id> [flags]
```

```
levelrail-cli apps environments env-get <id> [flags]
```

```
levelrail-cli apps environments env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]
```

```
levelrail-cli apps environments list <project-id> [flags]
```

```
levelrail-cli apps environments update <id> --protected=true|false [flags]
```

```
levelrail-cli apps exec <name> -- <command> [args...] [flags]
```

```
levelrail-cli apps git-source get <name> [flags]
```
show an app's connected repo

```
levelrail-cli apps images <name> [flags]
```

```
levelrail-cli apps log-drain get <name> [flags]
```
show an app's configured log drain

```
levelrail-cli apps logs <name> [flags]
```

```
levelrail-cli apps metrics <name> --metric NAME [flags]
```

```
levelrail-cli apps overview [name ...] [flags]
```

```
levelrail-cli upgrade [--no-backup] [flags]
```

`upgrade` runs the preflight checks, takes a control plane backup and prints the upgrade command. It never upgrades by itself. See [Installing](installing.md#check-first-then-upgrade).

```
levelrail-cli apps moves list <name> [flags]
```
list every node-to-node move attempt for `<name>`, newest first

```
levelrail-cli apps moves get <name> <id> [flags]
```
show one move attempt's step-by-step progress

```
levelrail-cli apps organizations clear-project <project-id> [flags]
```

```
levelrail-cli apps organizations create --name NAME [flags]
```
create an organization

```
levelrail-cli apps organizations delete <id> [flags]
```

```
levelrail-cli apps organizations env-get <id> [flags]
```

```
levelrail-cli apps organizations env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]
```

```
levelrail-cli apps organizations get <id> [flags]
```

```
levelrail-cli apps organizations list [flags]
```

```
levelrail-cli apps organizations set-project <project-id> <org-id> [flags]
```

```
levelrail-cli apps preview-env set <name> <key> --value VALUE [flags]
```
declare (or replace) a preview-specific env var override

```
levelrail-cli apps preview-env clear <name> <key> [flags]
```
remove a preview-specific env var override

```
levelrail-cli apps branch-env list <name> [flags]
```
list an app's branch-scoped env var overrides

```
levelrail-cli apps branch-env set <name> <key> --branch PATTERN --value VALUE [--secret] [flags]
```
declare (or replace) a branch-scoped env var override, applied only when a preview's own branch matches PATTERN

```
levelrail-cli apps branch-env clear <name> <id> [flags]
```
remove one branch-scoped override by its id (from `list` or `set`)

```
levelrail-cli apps previews list <app-name> [flags]
```
list active previews for an app

```
levelrail-cli apps previews pr-status enable <app-name> [flags]
```

```
levelrail-cli apps previews sweep [flags]
```

```
levelrail-cli apps previews teardown <app-name> <pr-number> [flags]
```

```
levelrail-cli apps projects create --name NAME [flags]
```
create a project

```
levelrail-cli apps projects delete <id> [flags]
```

```
levelrail-cli apps projects env-get <id> [flags]
```

```
levelrail-cli apps projects env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]
```

```
levelrail-cli apps projects get <id> [flags]
```

```
levelrail-cli apps projects list [flags]
```

```
levelrail-cli apps projects restart <id> [flags]
```

```
levelrail-cli apps projects start <id> [flags]
```

```
levelrail-cli apps projects stop <id> [flags]
```

```
levelrail-cli apps promote <name> --to ENVIRONMENT_ID [--target NAME] [--preview] [flags]
```

```
levelrail-cli apps restart <name> [flags]
```

```
levelrail-cli apps rollback <name> --image IMAGE [flags]
```

```
levelrail-cli apps scheduled-tasks create <app> --schedule CRON [--disabled] -- <command> [args...]
```

```
levelrail-cli apps scheduled-tasks delete <app> <id> [flags]
```

```
levelrail-cli apps scheduled-tasks get <app> <id> [flags]
```

```
levelrail-cli apps scheduled-tasks list <app> [flags]
```

```
levelrail-cli apps scheduled-tasks run <app> <id> [flags]
```

```
levelrail-cli apps scheduled-tasks update <app> <id> --schedule CRON [--disabled] -- <command> [args...]
```

```
levelrail-cli apps env import <name> --file .env [--dry-run] [--keep-existing] [--apply] [flags]
```
merge a .env file into an app's plain env vars, printing which keys are new, changed or unchanged (keys that are secrets are skipped); prints how many changes are pending, or restarts the app right away with `--apply`

```
levelrail-cli apps env export <name> [--out FILE] [flags]
```
write an app's env vars as .env text; secret keys are written empty with a comment, never with a value

```
levelrail-cli apps secrets list <name> [flags]
```
list an app's secret keys and their locked state

```
levelrail-cli apps secrets set <name> <key> <value> [--apply] [flags]
```
set or rotate one secret's encrypted value and declare the key as secret-backed so it is injected; `--apply` restarts the app now

```
levelrail-cli apps secrets delete <name> <key> [--force] [--apply] [flags]
```
delete a secret's value and stop declaring the key

```
levelrail-cli apps secrets set <name> --env-file <path> [flags]
```
bulk-import every key in a .env-format file as its own secret

```
levelrail-cli apps secrets lock <name> <key> --locked=true|false [flags]
```
toggle a secret's overwrite guard

```
levelrail-cli apps set-environment <name> <environment-id> [flags]
```

```
levelrail-cli apps set-project <name> <project-id> [flags]
```

```
levelrail-cli apps start <name> [flags]
```

```
levelrail-cli apps stop <name> [flags]
```

```
levelrail-cli apps storage set <name> --storage-target-id ID [flags]
```
attach a connected bucket as object storage

```
levelrail-cli apps vault-env set <name> <key> --path PATH --key FIELD [flags]
```
declare (or replace) a Vault-sourced env var

```
levelrail-cli apps vault-env clear <name> <key> [flags]
```
remove a Vault-sourced env var declaration

```
levelrail-cli apps webhook-deliveries list <app-name> [flags]
```
list recent inbound webhook requests

```
levelrail-cli apps webhook-deliveries replay <app-name> <delivery-id> [flags]
```

```
levelrail-cli apps tag <name> <tag> [flags]
```
attach a tag (by name) to an app, creating the tag if it doesn't exist

```
levelrail-cli apps untag <name> <tag> [flags]
```
detach a tag (by name) from an app

## Tags

```
levelrail-cli tags list [flags]
```

```
levelrail-cli tags create --name NAME [flags]
```

```
levelrail-cli tags delete <name> [flags]
```
delete a tag, identified by name (detaches from all apps)

```
levelrail-cli tags apps <name> [flags]
```
list every app attached to a tag, identified by name

## Pipelines

```
levelrail-cli pipelines list <app> [flags]
```

```
levelrail-cli pipelines validate <file> [--json]
```
validate a pipeline file locally, no API call, exit status 2 when it has problems

```
levelrail-cli pipelines save <app> <file-or-repo-dir> [--name N] [flags]
```
create or update pipelines from one file, or from every file in a repository's pipeline directory

```
levelrail-cli pipelines delete <app> <name> [flags]
```

```
levelrail-cli pipelines run <app> <name> [--ref R] [--sha S] [--input k=v]... [--follow] [flags]
```

```
levelrail-cli pipelines runs <app> [<run-id>] [--pipeline N] [--limit N] [flags]
```
list runs, or show one run's jobs, steps, and approval gates

```
levelrail-cli pipelines logs <app> <run-id> [--job KEY] [--follow] [flags]
```

```
levelrail-cli pipelines cancel <app> <run-id> [flags]
```

```
levelrail-cli pipelines approve <app> <run-id> [--reject] [--comment TEXT] [--approval ID] [flags]
```
decide approval gates, or release a run held for approval

```
levelrail-cli pipelines sync <app> [--repo-truth=true|false] [flags]
```
sync pipeline files from the repository now, or set repository as source of truth

```
levelrail-cli pipelines triggers <app> [flags]
```
why recent git events did or did not start runs
## Lb

```
levelrail-cli lb list [--state balancing|degraded|none] [--search Q] [flags]
```
every load balancer across apps with state and healthy upstream counts

```
levelrail-cli lb show <app> [flags]
```
show an app's load balancer config

```
levelrail-cli lb set <app> [--algorithm ...] [flags]
```
create or change the load balancer, only the flags you pass change

```
levelrail-cli lb clear <app> [flags]
```
remove the load balancer, back to a single upstream

```
levelrail-cli lb status <app> [flags]
```
live upstream table: state, weight, active requests, failures

```
levelrail-cli lb export <app> --format terraform|cdk|cloudformation|caddy|caddy-json [--out FILE]
```
generate an infrastructure-as-code definition, no cloud API calls

```
levelrail-cli lb import <app> --file app.yaml [--service S] [flags]
```
load the `loadbalancer:` block of an app.yaml

## Preview

```
levelrail-cli preview status <app> [flags]
```
show the app's deploy preview settings, storage used and latest result

```
levelrail-cli preview enable <app> [--mode metadata|screenshot] [--path /] [--wait-ms N] [flags]
```
turn deploy previews on for the app: `metadata` reads the page title and social image (no browser), `screenshot` (the default here) runs a browser container per deploy

```
levelrail-cli preview disable <app> [flags]
```
turn deploy previews off for the app

```
levelrail-cli preview capture <app> [flags]
```
recapture the current release now

```
levelrail-cli preview prune <app> [--all] [flags]
```
delete old previews now, or every preview of the app with `--all`

## Supply chain

```
levelrail-cli apps sbom <app> [deploy-id] [--download] [--file PATH] [flags]
```
show a deploy's software bill of materials (newest deploy with one by default), or print or save the raw SPDX or CycloneDX document

```
levelrail-cli apps scan enable <app> [flags]
```
turn vulnerability scanning on for the app; the first scan pulls the scanner image

```
levelrail-cli apps scan disable <app> [flags]
```
turn scanning off and reset the gate

```
levelrail-cli apps scan status <app> [deploy-id] [flags]
```
show the scan settings and the latest scan result

```
levelrail-cli apps scan run <app> [deploy-id] [flags]
```
scan a deploy's SBOM now

```
levelrail-cli apps scan gate <app> off|warn|block_on_critical [flags]
```
choose what a scan may do to a release; `block_on_critical` keeps the previous release serving

```
levelrail-cli apps scan override <app> --reason TEXT [flags]
```
let the next blocked release through once, with a recorded reason

## Databases

```
levelrail-cli databases clear-project <name> [flags]
```

```
levelrail-cli databases create --name NAME --engine ENGINE --version VERSION [flags]
```

```
levelrail-cli databases create [flags]
```
create a managed database

```
levelrail-cli databases status <name> [flags]
```
show a database's current reconcile conditions (useful when it exists but is not running yet)

```
levelrail-cli databases delete <name> [flags]
```

```
levelrail-cli databases metrics <name> --metric NAME [flags]
```

```
levelrail-cli databases public-access set <name> [--port N] [--bind-address ADDR] [flags]
```
expose a database on a host port; --bind-address is "private" (default), "public", or a literal IP

```
levelrail-cli databases public-access clear <name> [flags]
```

```
levelrail-cli databases set-resources <name> [--memory 512Mi] [--cpu 0.5] [flags]
```
applies memory/CPU limits to an already-created database, replacing whatever was set before (full replace, not a patch)

```
levelrail-cli databases set-project <name> <project-id> [flags]
```

```
levelrail-cli databases start <name> [flags]
```

```
levelrail-cli databases stop <name> [flags]
```

## Models

```
levelrail-cli models list [flags]
```
list AI models with their status

```
levelrail-cli models get <name> [flags]
```
show one model, its status and OpenAI-compatible base URL

```
levelrail-cli models deploy --name NAME --engine ENGINE --model MODEL [flags]
```
deploy a model on a GPU node; prints the API key once. Flags: --node, --gpus, --gpu-devices, --context, --quantization, --domain, --hf-token-from-env

```
levelrail-cli models logs <name> [flags]
```
search stored engine logs, or --follow to stream download and load progress live

```
levelrail-cli models delete <name> [flags]
```
remove a model; the downloaded weights volume is kept

```
levelrail-cli models restart <name> [flags]
```
recreate the engine container

```
levelrail-cli models rotate-key <name> [flags]
```
issue a new API key, printed once

```
levelrail-cli models gpus [flags]
```
list GPU nodes with driver, VRAM, usage and nvidia runtime status

```
levelrail-cli models preflight <repo> [flags]
```
check a Hugging Face repo before deploying: access, size, quantizations with a fit estimate, free disk

```
levelrail-cli models cache list|prune [flags]
```
list cached model weights per node, or prune unused ones (`--dry-run` first)

See [AI models](ai-models.md).

## Auth

```
levelrail-cli auth 2fa disable --code CODE|--recovery-code CODE [flags]
```

```
levelrail-cli auth 2fa enable --code CODE [flags]
```

```
levelrail-cli auth 2fa recovery-codes --code CODE [flags]
```

```
levelrail-cli auth 2fa setup [flags]
```

```
levelrail-cli auth 2fa status [flags]
```
show whether two-factor auth is enabled

```
levelrail-cli auth login [flags]
```
authenticate and persist a new API token

```
levelrail-cli auth whoami [flags]
```

## Profile

```
levelrail-cli profile list [flags]
```
list configured credentials profiles

## Tokens

```
levelrail-cli tokens create --name NAME --abilities LIST [--agent NAME] [--agent-description TEXT] [flags]
```
mint a new API token; `--agent` labels it as issued to an AI agent so audit entries record the agent name

```
levelrail-cli tokens list [flags]
```

```
levelrail-cli tokens revoke <id> [flags]
```

## Domains

```
levelrail-cli domains basic-auth get <app> <domain> [flags]
```
show a domain's basic auth state

```
levelrail-cli domains check <app> <domain> [flags]
```

```
levelrail-cli domains cloudflare-dns get [flags]
```
show the current settings

```
levelrail-cli domains route53-dns get [flags]
```
show the current settings

```
levelrail-cli domains list [flags]
```
list every app's domains in one call

```
levelrail-cli domains maintenance get <app> <domain> [flags]
```
show a domain's maintenance state

```
levelrail-cli domains redirect get <app> <domain> [flags]
```
show a domain's redirect state

```
levelrail-cli domains tls-cert get <app> <domain> [flags]
```
show a domain's BYO certificate state

```
levelrail-cli domains waf get <app> <domain> [flags]
```
show a domain's WAF and rate-limit state

```
levelrail-cli domains error-pages get <app> <domain> [--code N] [flags]
```
 show a domain's custom error pages

## Backups

```
levelrail-cli backups list <database> [flags]
```
list backup history for a database

```
levelrail-cli backups list-all [flags]
```
list backup history across every database and app volume instance-wide

```
levelrail-cli backups restore <database> --backup ID [--confirm NAME] [flags]
```

```
levelrail-cli backups restore-as-new <database> --backup ID --new-name NAME [flags]
```

```
levelrail-cli backups restores <database> [flags]
```
list restore attempt history for a database

```
levelrail-cli backups clone-restores <database> [flags]
```
list restore-as-new attempt history for a database

```
levelrail-cli backups schedule set <database> --target ID --cron EXPR [flags]
```
 configure a recurring backup

```
levelrail-cli backups trigger <database> --target ID [flags]
```

```
levelrail-cli backups verifications <database> --backup ID [flags]
```

```
levelrail-cli backups verify <database> --backup ID [flags]
```

## App Volume Backups

```
levelrail-cli app-volume-backups list <app> <volume> [flags]
```
list backup history for an app's named volume

```
levelrail-cli app-volume-backups restore <app> <volume> --backup ID [--confirm APP/VOLUME] [flags]
```

```
levelrail-cli app-volume-backups restore-as-new <app> <volume> --backup ID [--new-volume-name NAME] [flags]
```

```
levelrail-cli app-volume-backups restores <app> <volume> [flags]
```
list restore attempt history for an app's named volume

```
levelrail-cli app-volume-backups clone-restores <app> <volume> [flags]
```
list restore-as-new attempt history for an app's named volume

```
levelrail-cli app-volume-backups schedule set <app> <volume> --target ID --cron EXPR [flags]
```
 configure a recurring backup

```
levelrail-cli app-volume-backups trigger <app> <volume> --target ID [flags]
```

```
levelrail-cli app-volume-backups verifications <app> <volume> --backup ID [flags]
```

```
levelrail-cli app-volume-backups verify <app> <volume> --backup ID [flags]
```

## PITR Restores

```
levelrail-cli pitr restores <database> [flags]
```
list point-in-time restore attempts for a database (base backup, target time, status, error)

## Build

```
levelrail-cli build detect --repo-url URL [--ref REF] [flags]
```
show which framework the builder detects for a public repo, without running a build. Prints `no framework detected` (exit 0) when nothing matches.

```
levelrail-cli build branches --repo-url URL [flags]
```
list the branches a public repo advertises. Private or unreachable repos fail with an API error.

## Cloudflare Tunnel

```
levelrail-cli cloudflare-tunnel get [flags]
```
show the current settings and connection status

## Vault

```
levelrail-cli vault get [flags]
```
show the current external Vault integration settings

```
levelrail-cli vault set --address URL --auth-method token|approle [flags]
```
configure and enable resolving app secrets from an external HashiCorp Vault instance

```
levelrail-cli vault disconnect [flags]
```
disable and forget the stored credential

## Channels

```
levelrail-cli channels create --name NAME --kind KIND --notify-url URL [flags]
```

```
levelrail-cli channels delete <id> [flags]
```

```
levelrail-cli channels deliveries <id> [flags]
```

```
levelrail-cli channels list [flags]
```
list connected notification channels

```
levelrail-cli channels test <id> [flags]
```

```
levelrail-cli channels update <id> --name NAME --kind KIND --notify-url URL [flags]
```

## Backup Targets

```
levelrail-cli backup-targets create --name NAME --provider PROVIDER --bucket BUCKET --access-key-id ID --secret-access-key KEY [flags]
```

```
levelrail-cli backup-targets delete <id> [flags]
```

```
levelrail-cli backup-targets get <id> [flags]
```

```
levelrail-cli backup-targets list [flags]
```
list connected backup targets

```
levelrail-cli backup-targets test <id> [flags]
```

```
levelrail-cli backup-targets update <id> --name NAME --provider PROVIDER --bucket BUCKET [flags]
```

## Storage

```
levelrail-cli storage providers
```
list provider presets (aws, r2, b2, minio, wasabi, custom)

```
levelrail-cli storage list
```

```
levelrail-cli storage add --name N --provider P --bucket B --access-key-id ID --secret-access-key KEY [flags]
```

```
levelrail-cli storage test <id>
```
write, read back and delete a probe object

```
levelrail-cli storage delete <id>
```

## Logs

```
levelrail-cli logs archive set --target ID [--app NAME] [--interval 1h] [--retention-days N] [--disable]
```

```
levelrail-cli logs archive status
```

```
levelrail-cli logs archive remove [--app NAME]
```

```
levelrail-cli logs dump --target ID --from TIME [--to TIME] [--app NAME] [--wait]
```

```
levelrail-cli logs ls --target ID [--app NAME]
```

```
levelrail-cli logs fetch --target ID --key KEY [--out FILE]
```

```
levelrail-cli logs query <app> [--level LEVEL] [--since 30m] [--until T] [--deploy ID] [--text PHRASE] [--max-lines N] [--max-bytes N] [flags]
```
capped excerpt of an app's newest matching log lines with match counts and a truncation notice; the byte cap defaults to 8 KB or `APP_MCP_LOG_MAX_BYTES`

## Registry Credentials

```
levelrail-cli registry-credentials create --name NAME --registry-host HOST --username USER --password PASS [flags]
```

```
levelrail-cli registry-credentials delete <id> [flags]
```

```
levelrail-cli registry-credentials get <id> [flags]
```

```
levelrail-cli registry-credentials list [flags]
```
list connected registry credentials

```
levelrail-cli registry-credentials repositories <id> [flags]
```

```
levelrail-cli registry-credentials tags <id> <repository> [flags]
```

```
levelrail-cli registry-credentials test <id> [flags]
```

```
levelrail-cli registry-credentials update <id> --name NAME --registry-host HOST --username USER [flags]
```

## Registry

```
levelrail-cli registry status [flags]
```
show the current settings and container status

## Flags

```
levelrail-cli flags create <app> --key KEY --name NAME [--description DESC] [--disabled] [--rollout PERCENT] [flags]
```

```
levelrail-cli flags delete <app> <id> [flags]
```

```
levelrail-cli flags get <app> <id> [flags]
```

```
levelrail-cli flags list <app> [flags]
```

```
levelrail-cli flags set <app> <id> --name NAME [--description DESC] [--disabled] [--rollout PERCENT] [flags]
```

## Apply, Diff and Export

See [Platform as code](platform-as-code.md) for the document format, secrets handling, prune rules and CI use.

```
levelrail-cli apply -f file|dir|- [--dry-run] [--exit-code] [--prune --source NAME] [--project P] [--yes] [--secret K=env:VAR] [--var NAME=VALUE] [--var-file PATH] [--allow-env NAME[,NAME...]] [--no-deploy] [--continue-on-error] [flags]
```
validate resource files, print the plan, and apply it through the API with your own permissions. Exit 0 no changes or applied, 1 error, 2 changes pending (with `--dry-run --exit-code`). <span v-pre>`${{ env.NAME }}`</span> placeholders are filled only from `--var`, `--var-file` or the names listed with `--allow-env` (a trailing `*` allows a prefix, but never covers credential looking names such as `AWS_*`, `GITHUB_TOKEN` or anything containing `TOKEN`, `SECRET`, `PASSW` or `_KEY`, which must be named exactly); an unresolved placeholder fails before anything is sent

```
levelrail-cli diff -f dir [flags]
```
drift between the files and live state, exits 2 when they differ

```
levelrail-cli export [--project P] [--app A] [-o dir|-] [--include-env-values=false] [flags]
```
write live state as stable resource files, never containing secret values. Secret looking values become <span v-pre>`${{ env.NAME }}`</span> placeholders; supply them at apply time with `--var`, `--var-file` or `--allow-env`

## Nodes

```
levelrail-cli nodes delete <id> [flags]
```

```
levelrail-cli nodes drain <id> [--target NODE-ID] [flags]
```

```
levelrail-cli nodes get <id> [flags]
```

```
levelrail-cli nodes health <id> [flags]
```

```
levelrail-cli nodes join-token [flags]
```

```
levelrail-cli nodes list [flags]
```

```
levelrail-cli nodes list [flags]
```
list every node

```
levelrail-cli nodes metrics <id> --metric NAME [flags]
```

```
levelrail-cli nodes patch-status <id> [flags]
```

```
levelrail-cli nodes events <id> [--limit N] [flags]
```

```
levelrail-cli nodes workloads <id> --accepts-app=BOOL --accepts-build=BOOL [flags]
```

```
levelrail-cli nodes reenroll-token <id> [flags]
```
mint a one-time token that re-issues a node's agent certificate, keeping its identity; prints the command to run on the node

```
levelrail-cli nodes revoke-cert <id> [flags]
```
revoke a node's agent certificate and close its session; only a re-enroll token brings it back

`nodes list` shows each node's certificate state and days left (CERT) and agent version (AGENT); `nodes get` adds expiry, last renewal, key origin, platform and commit.

## Status

```
levelrail-cli status [flags]
```

## Version

```
levelrail-cli version [flags]
```

::: details Audit Log and Audit Purge (administrative)

### Audit Log

```
levelrail-cli audit-log [flags]
```

Filter with `--agent <name>` (entries made with a token labeled with that agent name), `--search <text>` (case-insensitive substring across actor, ability, method, path and remote address) and `--failed` (status 400 or higher). Both are applied server side and carry into `--format csv` exports.

### Audit Purge

```
levelrail-cli audit-purge [flags]
```

:::

::: details Attention (troubleshooting)

### Attention

```
levelrail-cli attention [flags]
```

Lists everything that needs attention right now: failing apps, offline nodes, expired or expiring certificates, and doctor warnings or failures, critical first. It is the CLI side of the dashboard's Status page (`/status`). Exit code is 1 if any item is critical, 0 otherwise, so it works as a script gate. Supports `--json`, `--output json|table|text`, and `--query`.

:::

::: details Doctor (troubleshooting)

### Doctor

```
levelrail-cli doctor [flags]
```

:::

::: details Containers (low-level)

### Containers

```
levelrail-cli containers [flags]
```

:::

::: details System Maintenance (fleet-wide cleanup, requires an admin/root-scoped token)

### System Prune

```
levelrail-cli system-prune [flags]
```

Removes every stopped container, dangling image, and unused anonymous
volume or build cache not part of the reconciler's current desired
state, fleet-wide. Never touches a named volume (an app's storage
attachment, a database's data volume), even one that's actually
orphaned: see Orphaned Volumes below for those.

### Control Plane Backups

```
levelrail-cli control-plane-backups list [flags]
levelrail-cli control-plane-backups create [flags]
levelrail-cli control-plane-backups download <name> [--out FILE] [flags]
levelrail-cli control-plane-backups verify <name> [flags]
levelrail-cli control-plane-backups delete <name> [flags]
levelrail-cli control-plane-backups list --offbox [flags]
levelrail-cli control-plane-backups schedule show|set [flags]
levelrail-cli control-plane-backups run-now [--no-wait] [flags]
levelrail-cli control-plane-backups drill run|status [flags]
levelrail-cli control-plane-backups escrow [--out FILE] [--recipient KEY] [--upload] [--ack] [flags]
levelrail-cli control-plane-backups escrow ack [flags]
levelrail-cli control-plane-backups escrow open <file> --identity FILE [--extract DIR]
levelrail-cli control-plane-backups keys generate [--out FILE] [--hybrid]
```

Snapshots of the control plane's own database, stored under
`<data dir>/control-plane-backups/`. `create` takes one now (manual
snapshots are never auto-deleted), `download` saves one to `--out FILE`
(or raw bytes to stdout), `verify` re-checks the checksum, SQLite
integrity and schema version without restoring (exit 1 if any check
fails), `delete` removes one. Snapshots never contain
the master key. Restore is an offline server command, `levelrail
restore-db <file>`; see [Control plane backup and
restore](/control-plane-backup).

The `--offbox`, `schedule`, `run-now`, `drill`, `escrow` and `keys`
subcommands drive encrypted off-box backups: `keys generate` makes an age
key pair on your machine (private key to a `0600` file, public key to
stdout), `schedule set` changes only the flags you pass, `run-now` and
`drill run` wait for the run and exit 1 if it failed, `drill status` exits
1 when the last drill failed or none has run, and `escrow` writes the master
key and agent CA key encrypted to your recipients (never uploaded unless `--upload`, and never
to the backup bucket). Restore an off-box backup on the server with
`levelrail restore --from <s3://... | file> --identity FILE [--dry-run]`.
See [Disaster recovery](/disaster-recovery).

### Orphaned Volumes

```
levelrail-cli volumes-orphaned [flags]
levelrail-cli volumes-orphaned-cleanup --names name1,name2 [flags]
```

Named Docker volumes (an app's storage attachment, a database's data
volume) survive `system-prune` even after the app or database that
created them is deleted, since Docker never removes a named volume on
its own. `volumes-orphaned` lists every one this instance created that
no current app, database, or storage attachment references any more.
`volumes-orphaned-cleanup` removes exactly the volumes named with
`--names` (comma-separated), after the control plane re-confirms each
one is still genuinely orphaned; there is no flag that deletes every
currently orphaned volume sight unseen, review the list first.

:::

## Users

```
levelrail-cli invites create --email EMAIL --role ROLE [flags]
```
 invite a new teammate

```
levelrail-cli users create --email EMAIL --password PASSWORD --role ROLE [flags]
```

```
levelrail-cli users delete <id> [flags]
```

```
levelrail-cli users list [flags]
```
list every user

```
levelrail-cli users roles [flags]
```

```
levelrail-cli users set-abilities <id> --role ROLE [flags]
```

## Iam

```
levelrail-cli iam policies <verb> [flags]
```

```
levelrail-cli iam policies attach <id> --principal-type TYPE --principal-id ID [flags]
```

```
levelrail-cli iam policies attachments <id> [flags]
```

```
levelrail-cli iam policies create --name NAME --document DOC [flags]
```
create a policy

```
levelrail-cli iam policies delete <id> [flags]
```

```
levelrail-cli iam policies detach <id> --principal-type TYPE --principal-id ID [flags]
```

```
levelrail-cli iam policies get <id> [flags]
```

```
levelrail-cli iam policies list [flags]
```

```
levelrail-cli iam policies update <id> --name NAME --document DOC [flags]
```

## Secrets

```
levelrail-cli secrets rotate-master-key --new-key-file PATH [flags]
```

```
levelrail-cli secrets binding-status [flags]
```
count stored secret values not yet bound to their slot

```
levelrail-cli secrets rebind [flags]
```
bind every legacy secret value to its slot, safe to rerun

::: details Migrate (one-time platform migration)

### Migrate

```
levelrail-cli migrate caprover --url URL --token TOKEN [flags]
```

```
levelrail-cli migrate coolify --url URL --token TOKEN [flags]
```
migrate apps from a Coolify instance

```
levelrail-cli migrate dokploy --url URL --token TOKEN [flags]
```

:::

::: details Import (from another platform)

### Import platform

```
levelrail-cli import platform coolify|dokploy|caprover --url URL [flags]
```
read apps and databases from another platform and create them here; use `--dry-run` first, see [migrating from Coolify, Dokploy or CapRover](migrating-from-coolify-dokploy-and-caprover.md)

:::

::: details Completion (shell setup)

```
levelrail-cli completion bash
```
print a bash completion script

:::

::: details Settings (system configuration)

### Settings

```
levelrail-cli settings email get [flags]
```

```
levelrail-cli settings ingress get [flags]
```

```
levelrail-cli settings dashboard-url get [flags]
```
shows the public dashboard URL

```
levelrail-cli settings dashboard-url set --url URL [flags]
```
sets it; once it is `https://`, sign-in over plain HTTP is refused (`--url ""` clears it)

```
levelrail-cli settings oauth list [flags]
```
show every OAuth sign-in provider's current settings

```
levelrail-cli settings ai-assistant get [flags]
```
shows the current AI assistant settings (the key itself is never returned, only whether one is stored)

```
levelrail-cli settings ai-assistant set --model NAME --api-key KEY [flags]
```
configures the AI assistant

```
levelrail-cli settings ai-assistant clear [flags]
```
clears the stored key and resets provider/model

:::

::: details Git Integrations (Github, Gitlab, Bitbucket, Gitea setup)

```
levelrail-cli git-providers [flags]
```
connection status and capabilities (list branches, register a webhook, authenticated clone) for github, gitlab, bitbucket, and gitea in one call

### Github App

```
levelrail-cli github-app status [flags]
```

```
levelrail-cli github-app disconnect [flags]
```
forgets the stored connection locally; does not uninstall or delete the App on GitHub's own side

```
levelrail-cli github-app repos [flags]
```
list repos every connected installation can access

```
levelrail-cli github-app installations list [flags]
```
list every connected account/org

```
levelrail-cli github-app installations add [flags]
```
print the URL to install the App on another account/org; does not open a browser or drive the install flow itself

```
levelrail-cli github-app installations remove <id> [flags]
```
disconnect one account/org; refused (409) while a git source still points at a repo under it

### Gitlab App

```
levelrail-cli gitlab-app status [flags]
```

```
levelrail-cli gitlab-app disconnect [flags]
```
forgets the stored connection locally; does not revoke the token or delete the Application on GitLab's own side

```
levelrail-cli gitlab-app projects [flags]
```
list projects the connected account can access

### Bitbucket App

```
levelrail-cli bitbucket-app status [flags]
```

```
levelrail-cli bitbucket-app disconnect [flags]
```
forgets the stored connection locally; does not revoke the token or delete the consumer on Bitbucket's own side

```
levelrail-cli bitbucket-app repos [flags]
```
list repos the connected account can access

### Gitea App

```
levelrail-cli gitea-app status [flags]
```

```
levelrail-cli gitea-app disconnect [flags]
```
forgets the stored connection locally; does not revoke the token or delete the application on Gitea's own side

```
levelrail-cli gitea-app repos [flags]
```
list repos the connected account can access

:::

## Templates

```
levelrail-cli templates list [--custom] [flags]
```
browse the curated service catalog, or `--custom` for your own saved templates

```
levelrail-cli templates delete <id> [flags]
```
deletes a custom template (see `apps save-as-template`); the built-in catalog is read-only

## Static Sites

```
levelrail-cli static-sites list [flags]
```

