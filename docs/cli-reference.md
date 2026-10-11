---
description: Complete reference of all Levelrail CLI commands, organized by group with examples and typical use cases.
---

# Levelrail CLI Reference

`levelrail-cli` is a scriptable client for the control plane's HTTP API. Everything it does goes through `/api/v1`, with the same tokens and permissions as the dashboard, so it needs no SSH key and no open port on your servers. This page lists every command group with its subcommands and the flags that matter most. Run `levelrail-cli` with no arguments for the command list, or `levelrail-cli <command> <subcommand> -h` for any command's own flags.

Commands are shown as `levelrail-cli ...`. If you rename the binary, the command name follows it (the CLI reads its name from `os.Args[0]`). The server is a separate binary, `levelrail`, which also has a few maintenance commands such as `setup-token`, `restore-snapshot`, `restore-db`, `recover-admin`, `upgrade-note` (record who swapped the binary by hand), and `healthcheck`. Those are covered in [Installing](installing.md) and [Disaster recovery](/disaster-recovery).

## Install and sign in

<CopyCommand command="curl -fsSL https://levelrail.com/install-cli.sh | sh" />

```bash
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
| 6 | Deploy timeout: `apps wait` or `apps deploys wait` gave up before the deploy converged |
| 7 | `apps deploys wait` ended on a deploy that failed, was canceled, superseded or blocked |

Codes 3 and 4 are broad on purpose so existing scripts keep working; the JSON error object carries the finer distinction.

## Scripting: `--debug`

`--debug` works anywhere on the command line (before or after the subcommand) and traces every outgoing request this invocation makes to stderr: method, URL, and the response status and timing, one line per call. It never prints request/response headers or bodies, so `Authorization` and any token value never appear, even in debug mode; a token that somehow ended up in a URL's query string is also redacted. stdout is untouched, so `--debug` composes with `--json`/`--query` for scripting.

```
levelrail-cli apps list --debug --api-url http://10.0.0.5:8080
```
```
DEBUG: GET http://10.0.0.5:8080/api/v1/apps -> 200 OK (42ms)
```


## Experimental command groups

Some groups are off by default and hidden from `--help` and shell completion until the shell you run the CLI from sets `APP_EXPERIMENTAL`. This page marks them. The keys are `ai-chat` (`ai`, `settings ai-assistant`), `ai-models` (`models`), `load-balancer` (`lb`), `iac` (`apply`, `diff`, `export`) and `cloudflare-tunnel` (`cloudflare-tunnel`). See [Experimental features](experimental-features.md).

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

`apps list` prints every app the caller can read as a bare JSON array. On a fleet large enough that matters, add `--max-items N` to cap the page size; the response becomes `{"items": [...], "next_token": "...", "total_count": N}` instead, and an empty `next_token` means there is no further page. Pass that `next_token` back in as `--starting-token` to fetch the next page (requires `--max-items`, since the server only pages a result when a limit is set):

```
levelrail-cli apps list --max-items 50 --json
levelrail-cli apps list --max-items 50 --starting-token 50 --json
```

Run `levelrail-cli apps <subcommand> -h` for the full flag list of any command below. Deploy lifecycle is covered in [Deploying and managing apps](deploying-apps.md).

### Create, inspect and list

```
levelrail-cli apps create --name NAME --image IMAGE --port PORT [flags]
levelrail-cli apps create --name NAME --port PORT --repo URL --image-repo REPO [flags]
levelrail-cli apps create --file app.yaml [flags]
levelrail-cli apps create --interactive
levelrail-cli apps list [--max-items N] [--starting-token T] [flags]
levelrail-cli apps get <name> [flags]
levelrail-cli apps status <name> [flags]
levelrail-cli apps overview [name ...] [flags]
levelrail-cli apps network <name> [flags]
levelrail-cli apps validate --file <app.yaml|compose.yaml> [flags]
levelrail-cli apps clone <name> <new-name> [flags]
levelrail-cli apps delete <name> [flags]
```

- `apps create` registers an existing image or triggers a real build from a git repository; `--file` reads an app spec, `--interactive` runs a step-by-step wizard. Required flags missing outside `--interactive` fail with an error instead of prompting.
- `apps status` shows an app's current reconcile conditions; `apps overview` shows CPU, memory, traffic and errors for every app (or the names given) in one request; `apps network` shows the live traffic path (container port, host port, running).
- `apps validate` parses and validates an app.yaml or a Docker Compose file locally, with no API call and no deploy. It prints the detected format, service count, and every non-blocking `notices` entry a real deploy would also surface.
- `apps delete` removes an app and stops its containers; if the node is unreachable the delete is reported as pending and retried.

### Deploy, roll back and scale

```
levelrail-cli apps deploy <name> --image IMAGE [flags]
levelrail-cli apps deploy-compose <name> --file compose.yaml [flags]
levelrail-cli apps deploy-spec <name> --file app.yaml --repo-url <url> --ref <ref> [flags]
levelrail-cli apps builds trigger <name> --repo URL --ref REF [flags]
levelrail-cli apps rollback <name> --image IMAGE [flags]
levelrail-cli apps promote <name> --to ENVIRONMENT_ID [--target NAME] [--preview] [flags]
levelrail-cli apps wait <name> [--attempt-id ID] [--timeout 5m] [--poll-interval 3s] [flags]
levelrail-cli apps restart <name> [flags]
levelrail-cli apps apply <name> [flags]
levelrail-cli apps stop <name> [flags]
levelrail-cli apps start <name> [flags]
levelrail-cli apps scale <name> --replicas N [--strategy rolling|recreate|blue-green] [flags]
levelrail-cli apps group <name> [flags]
levelrail-cli apps hook-runs <name> [flags]
levelrail-cli apps images <name> [flags]
levelrail-cli apps set-node <name> <node-id> [--with-volumes] [flags]
levelrail-cli apps clear-node <name> [--with-volumes] [flags]
levelrail-cli apps moves list <name> [flags]
levelrail-cli apps moves get <name> <id> [flags]
```

- `apps wait` polls until a deploy attempt converges and exits 0 on success, 5 if it converged as a failure, 6 on timeout. On success it says what happened: `rolled out`, `already up to date` (the deploy changed nothing) or `restarted`; `--json` carries the same as `outcome`.
- `apps deploy-spec` fans an app.yaml's `services:` map out into independent builds under one app; `apps group` shows a service's siblings under the same multi-service app.
- `apps builds trigger` builds an image from a git source and deploys it to an existing app.
- `apps promote` promotes the app's image onto a sibling app in another environment.
- `apps apply` restarts an app so saved env, secret and config changes reach the running container; it does nothing when nothing is pending.
- `apps scale` changes the replica count and/or the deploy strategy.
- `apps hook-runs` shows the most recent outcome of an app's pre and post deploy hooks; `apps images` lists the locally present image tags under the app's current image repo.
- `apps set-node` and `apps clear-node` move an app to another node, or back to the control plane's own local node. With `--with-volumes` the named volumes move too, and `apps moves` shows the history of those moves.

### Deploy history, approvals and automation

```
levelrail-cli apps deploys list <name> [flags]
levelrail-cli apps deploys show <name> [deploy-id] [flags]
levelrail-cli apps deploys wait <name> [deploy-id] [--timeout 10m] [--poll-interval 2s] [flags]
levelrail-cli apps deploys compare <name> --from ID [--to ID] [flags]
levelrail-cli apps deploys logs <name> <deploy-id> [flags]
levelrail-cli apps deploys steps <name> <deploy-id> [flags]
levelrail-cli apps deploys failed [--since 24h] [flags]
levelrail-cli apps deploys cancel <name> <deploy-id> [flags]
levelrail-cli apps deploys rollback-to <name> <deploy-id> [flags]
levelrail-cli apps timeline <name> [--limit N] [flags]
levelrail-cli apps diagnose <name> [--deploy ID] [--apply-fix N] [flags]
levelrail-cli apps preflight <name> [--require-env A,B] [flags]
levelrail-cli apps freeze show <app-name> [flags]
levelrail-cli apps freeze set <app-name> --cron EXPR --duration D [--timezone TZ] [--reason TEXT] [flags]
levelrail-cli apps freeze clear <app-name> [flags]
levelrail-cli apps schedule set <app> --cron EXPR --branch NAME [flags]
levelrail-cli apps schedule get <app> [flags]
levelrail-cli apps schedule history <app> [flags]
levelrail-cli apps cancel-superseded enable|disable|status <app-name> [flags]
levelrail-cli apps auto-rollback enable|disable|status <app-name> [flags]
levelrail-cli functions deploy|list|invoke|delete [flags]
levelrail-cli apps sleep enable|disable|status|wake <app-name> [flags]
levelrail-cli apps canary start|status|weight|promote|abort <app-name> [flags]
levelrail-cli apps auto-update enable|disable|status|check <app-name> [flags]
levelrail-cli apps auto-rollback-slo-burn set <app-name> off|auto|dry_run|pause_for_human [flags]
levelrail-cli apps auto-rollback-slo-burn status <app-name> [flags]
```

- `apps deploys list` is real, row per attempt deploy history, newest first. `apps deploys show` prints one attempt (the newest by default) with its structured failure: code, cause, failing step, redacted log excerpt, suggested fix, docs link and retryable. See [Deploy failures](deploy-failures.md).
- `apps deploys wait` blocks until one deploy is healthy, failed, canceled, superseded or blocked (a freeze window or approval) and prints the result with its failure. Exit 0 healthy, 7 failed, canceled, superseded or blocked, 6 timeout. Without a deploy id it resolves the newest deploy once and follows it by id.
- `apps deploys compare` diffs two deploy attempts, or one against the current live state. `apps deploys logs` prints one attempt's full build and log output to stdout. `apps deploys steps` streams the pipeline steps (detecting, building, pushing, deploying) until the attempt ends and exits non-zero if a step failed; an already finished attempt replays only a short summary.
- `apps deploys failed` lists every app's latest failed deploy in the window (default set by the server), with the image of its newest good deploy as a rollback target.
- `apps deploys cancel` cancels a queued or in-progress deploy before it cuts traffic. `apps deploys rollback-to` redeploys a past succeeded deploy's exact image, pinned by digest.
- `apps timeline` shows what happened to an app, newest first: deploys, rollbacks, restarts, env, secret and config changes (key names only, never values), scaling, stop and start.
- `apps diagnose` explains a failed deploy or crashloop and can apply a suggested fix with `--apply-fix N`. `apps preflight` runs pre-deploy checks (DNS, ports, disk, image, env).
- `apps freeze` manages deploy freeze windows: a window starts at every match of a 5 field cron expression, evaluated in `--timezone` (default UTC), and lasts `--duration`. While one is active, webhook, pipeline and released deploys are held and run when the window ends; a manual deploy needs `apps deploy --override-freeze --override-reason TEXT`. `set` replaces the app's windows with the one given. Fleet-wide windows are under `settings deploy-freeze`.
- `apps schedule` configures a recurring redeploy of a branch's latest commit on a cron schedule.
- `apps cancel-superseded` lets a newer queued deploy replace older queued ones of the same branch. Only queued deploys are replaced; a deploy that already started building is never canceled automatically.
- `functions` runs an image as a function: it sleeps when idle, and a request to a sleeping one waits through the cold start (see [Functions](functions.md)).
- `apps sleep` stops an app after it has had no requests for a set time and wakes it on the next request (see [Sleep when idle](sleep-when-idle.md)).
- `apps canary` runs a new image beside the app and sends it a share of traffic until you promote or abort it (see [Canary deploys](canary-deploys.md)).
- `apps auto-update` redeploys an app automatically when its image tag moves to a new digest (see [Image auto-update](image-auto-update.md)).
- `apps auto-rollback` opts an app into or out of automatic rollback when a crashloop alert fires. `apps auto-rollback-slo-burn` sets how the app reacts when an SLO burn rate alert fires.

```
levelrail-cli deployments list [--status a,b] [--app NAME] [--branch B] [--trigger T] [--environment E] [--since 24h] [--until T] [--q TEXT] [--live] [--pr N] [--limit N] [--cursor C] [flags]
levelrail-cli deployments summary [--window 24h] [flags]
levelrail-cli deployments watch [flags]
```
Deploys across every app you can read: a filterable newest first list (page size default 50, max 200, with `--cursor` paging), counts by status with failure rate and duration percentiles, and a live event stream (`--json` prints one object per event).

```
levelrail-cli deploy-approvals list [--status pending|all|approved|rejected|expired] [--service NAME] [flags]
levelrail-cli deploy-approvals get <id> [flags]
levelrail-cli deploy-approvals approve <id> [flags]
levelrail-cli deploy-approvals reject <id> [--reason TEXT] [flags]
```
A deploy or promote into an environment tagged protected becomes a pending approval that a different, sufficiently privileged user must approve first; the same user or token that requested it cannot decide it. `list` defaults to pending. `approve` runs the gated deploy or promote now; `reject` leaves the app's desired state untouched.

```
levelrail-cli apps deploy-notify-targets list <app> [flags]
levelrail-cli apps deploy-notify-targets create <app> --channel-id ID [flags]
levelrail-cli apps deploy-notify-targets delete <app> <id> [flags]
```
Attach an already connected notification channel (see [Channels](#channels)) so a deploy success or failure sends a message there.

### Domains, ports and traffic

```
levelrail-cli apps domains list <name> [flags]
levelrail-cli apps domains add <name> <domain>... [--dns auto|off|preview] [--replace] [--wait] [--timeout 2m] [flags]
levelrail-cli apps domains remove <name> <domain>... [--remove-dns] [flags]
levelrail-cli apps streams list <name> [flags]
levelrail-cli apps streams create <name> --container-port N --host-port N [--protocol tcp] [flags]
levelrail-cli apps streams delete <name> <id> [flags]
levelrail-cli apps egress get <name> [flags]
levelrail-cli apps egress set <name> --allow host:port [--allow host:port ...] [flags]
levelrail-cli apps egress clear <name> [flags]
levelrail-cli apps exec-access enable|disable|status <name> [flags]
```

- `apps domains add` creates the DNS record when a provider is connected (`--dns off` skips it, `--dns preview` shows it); `--wait` prints the go-live steps and exits non-zero when the domain is not live within `--timeout`. See [Add a domain and go live](domains-and-ingress.md#add-a-domain-and-go-live).
- `apps domains` shows or changes an app's domains; a domain already used by another app is refused and nothing is changed. Per-domain TLS, redirects, WAF and the like are under [Domains](#domains).
- `apps streams` forwards a host port to one container port as raw TCP (only `tcp` is supported). A stream change takes effect on the app's next container recreation, such as an image change or `apps restart`.
- `apps egress` restricts an app's outbound traffic to the declared `host:port` pairs; with nothing configured, egress is unrestricted. DNS and loopback traffic stay open regardless.
- `apps exec-access` is on by default. When disabled, `apps exec` and the interactive terminal are refused whatever abilities the token has.

### Run, logs and metrics

```
levelrail-cli apps exec <name> -- <command> [args...] [flags]
levelrail-cli apps logs <name> [flags]
levelrail-cli apps metrics <name> --metric NAME [flags]
levelrail-cli apps requests <name> [flags]
levelrail-cli apps resource-usage [flags]
levelrail-cli apps resource-recommendation <name> [flags]
levelrail-cli apps cost <name> [flags]
levelrail-cli apps health-score <name> [flags]
levelrail-cli apps log-drain get <name> [flags]
levelrail-cli apps log-drain set <name> --type TYPE --target TARGET [flags]
levelrail-cli apps log-drain clear <name> [flags]
levelrail-cli apps scheduled-tasks create <app> --schedule CRON [--disabled] -- <command> [args...]
levelrail-cli apps scheduled-tasks update <app> <id> --schedule CRON [--disabled] -- <command> [args...]
levelrail-cli apps scheduled-tasks list <app> [flags]
levelrail-cli apps scheduled-tasks get <app> <id> [flags]
levelrail-cli apps scheduled-tasks run <app> <id> [flags]
levelrail-cli apps scheduled-tasks delete <app> <id> [flags]
```

- `apps exec` runs a command in the app's container and exits with its real exit code.
- `apps logs` searches an app's stored log entries, or streams live with `--follow`. For a capped excerpt see `logs query` under [Logs](#logs).
- `apps requests` shows request rate, errors and latency from the ingress; `apps resource-usage` ranks every app by latest CPU, memory and network usage.
- `apps resource-recommendation` suggests memory and CPU limits from historical usage; `apps cost` estimates what the app's CPU and memory would cost under reference providers (not a real bill).
- `apps health-score` gives a synthesized pass, warn or fail readiness verdict across deploy health, security, resilience and observability.
- `apps log-drain` forwards container logs to an external HTTP or syslog sink, in addition to the built-in node-local log store.
- `apps scheduled-tasks` runs a command inside the app's running container on a cron schedule. The command is a real argv with no shell; put `--` before it when the command takes flags.

### Health probes, volumes and storage

```
levelrail-cli apps health get <name> [flags]
levelrail-cli apps health set <name> --probe readiness|liveness (--path PATH | --exec CMD | --preset NAME) [--scheme https] [--host HOST] [--tls-skip-verify] [--follow-redirects true|false] [--expected-status 200-399] [--interval 5s] [--timeout 2s] [--failures 3] [--ready-timeout 90s] [flags]
levelrail-cli apps health clear <name> [--probe readiness|liveness] [flags]
levelrail-cli apps health discover <name>
levelrail-cli apps volumes get <name> [flags]
levelrail-cli apps volumes attach <name> --name NAME --path PATH [flags]
levelrail-cli apps volumes detach <name> --name NAME [flags]
levelrail-cli apps storage set <name> --storage-target-id ID [flags]
levelrail-cli apps storage clear <name> [flags]
levelrail-cli apps build-cache show <name>|--global
levelrail-cli apps build-cache set <name>|--global --target ID [--mode min|max] [--disable]
levelrail-cli apps build-cache clear <name>
levelrail-cli apps build-cache remove <name>|--global
```

- `apps health set` sets one probe and keeps the other. A probe is an HTTP(S) check or a command run inside the container (exit 0 is healthy). `--preset` fills path, interval, timeout and failures from a shortcut (`healthz`, `health`, `api-health`, `ping`, `status`); explicit flags win. `apps health discover` probes well-known paths and reports what each one did.
- `apps volumes` manages named Docker volumes outside a redeploy. The name is lowercase letters, numbers and hyphens starting with a letter; the path is where it mounts in the container.
- `apps storage set` attaches a connected bucket as object storage and injects its credentials as `S3_*` env vars at container create time. See [Object storage](object-storage.md).
- `apps build-cache` configures the BuildKit remote cache on a storage destination. `--global` applies to every app without its own setting. `clear` is bounded per call; run it again while it reports more to delete. A cache problem never fails a build.

### Environment, secrets and Vault

```
levelrail-cli apps env import <name> --file .env [--dry-run] [--keep-existing] [--apply] [flags]
levelrail-cli apps env export <name> [--out FILE] [flags]
levelrail-cli apps env diff <app-a> <app-b> [flags]
levelrail-cli apps secrets list <name> [flags]
levelrail-cli apps secrets set <name> <key> <value> [--apply] [flags]
levelrail-cli apps secrets set <name> --env-file <path> [flags]
levelrail-cli apps secrets delete <name> <key> [--force] [--apply] [flags]
levelrail-cli apps secrets lock <name> <key> --locked=true|false [flags]
levelrail-cli apps vault-env set <name> <key> --path PATH --key FIELD [flags]
levelrail-cli apps vault-env clear <name> <key> [flags]
levelrail-cli apps preview-env set <name> <key> --value VALUE [flags]
levelrail-cli apps preview-env clear <name> <key> [flags]
levelrail-cli apps branch-env list <name> [flags]
levelrail-cli apps branch-env set <name> <key> --branch PATTERN --value VALUE [--secret] [flags]
levelrail-cli apps branch-env clear <name> <id> [flags]
```

- `apps env import` merges a .env file into an app's plain env vars and prints which keys are new, changed or unchanged. Keys that are already secrets are skipped. It reports how many changes are pending, or restarts the app right away with `--apply`.
- `apps env diff` compares two apps' env vars, for example staging against production. Secret values are never read: secrets are compared by key only, and the dashboard has the same comparison on an app's Environment page.
- `apps env export` writes plain env vars as .env text; secret keys are written empty with a comment, never with a value.
- `apps secrets set` sets or rotates one secret's encrypted value and declares the key as secret backed so it is injected; `--apply` restarts the app now. With `--env-file` it bulk imports every key in a .env format file as its own secret. Values are never returned: `list` shows key names and locked state only. `lock` toggles a secret's overwrite guard.
- `apps vault-env` declares an env var resolved live from the configured external Vault (see [Vault](#vault)); only the reference is stored.
- `apps preview-env` and `apps branch-env` override an env var for previews only. A branch override applies when the preview's own branch matches PATTERN (an exact name or a glob such as `release/*`); `--secret` stores it envelope encrypted. Neither affects the app's own running deploy.

### Source, previews and webhooks

```
levelrail-cli apps git-source get <name> [flags]
levelrail-cli apps git-source set <name> --repo-url URL [flags]
levelrail-cli apps git-source settings <name> [flags]
levelrail-cli apps git-source rotate-secret <name> [flags]
levelrail-cli apps git-source delete <name> [flags]
levelrail-cli apps webhook-deliveries list <app-name> [flags]
levelrail-cli apps webhook-deliveries replay <app-name> <delivery-id> [flags]
levelrail-cli apps previews list [app-name] [flags]
levelrail-cli apps previews limits [app-name] [flags]
levelrail-cli apps previews enable <app-name> [flags]
levelrail-cli apps previews disable <app-name> [flags]
levelrail-cli apps previews approve <app-name> <pr-number> --yes
levelrail-cli apps previews teardown <app-name> <pr-number> [flags]
levelrail-cli apps previews pr-status enable <app-name> [flags]
levelrail-cli apps previews pr-status disable <app-name> [flags]
levelrail-cli apps previews sweep [flags]
```

- `apps git-source` connects a repo for auto deploy on push; `settings` sets push path filters and forge status reporting; `rotate-secret` mints a fresh webhook secret, shown once.
- `apps webhook-deliveries` lists recent inbound webhook requests, verified or not. `replay` re-runs a stored delivery's payload and can trigger a real build and deploy.
- `apps previews` manages preview environments per pull request, see [Deploy previews](deploy-previews.md). `limits` shows or changes preview caps, fork policy and TTL; `approve` deploys a held fork pull request once; `teardown` removes one PR's preview now; `sweep` tears down every stale preview across all apps now; `pr-status` toggles the PR comment and commit status per preview deploy.

### Supply chain

```
levelrail-cli apps sbom <app> [deploy-id] [--download] [--file PATH] [flags]
levelrail-cli apps scan enable <app> [flags]
levelrail-cli apps scan disable <app> [flags]
levelrail-cli apps scan status <app> [deploy-id] [flags]
levelrail-cli apps scan run <app> [deploy-id] [flags]
levelrail-cli apps scan gate <app> off|warn|block_on_critical [flags]
levelrail-cli apps scan override <app> --reason TEXT [flags]
```

`apps sbom` shows a deploy's software bill of materials (newest deploy with one by default), or prints or saves the raw SPDX or CycloneDX document. Scanning is off by default and needs `APP_BUILD_ATTEST=true` on the server so builds produce an SBOM; a server can refuse every scan with `APP_SCAN_ENABLED=false`. `gate block_on_critical` keeps the previous release serving; `override` lets the next blocked release through once, with a recorded reason. See [Supply chain](supply-chain.md).

### Alerts, connections and templates

```
levelrail-cli apps alerts list <app> [flags]
levelrail-cli apps alerts create <app> --kind KIND [flags]
levelrail-cli apps alerts update <app> <id> --kind KIND [flags]
levelrail-cli apps alerts delete <app> <id> [flags]
levelrail-cli apps alerts slo <app> [--slo 99.9] [--slo-latency-ms N]
```

`apps alerts create` takes one of these kinds: `threshold` (needs `--metric`, `--comparator`, `--threshold`), `crashloop` (`--restart-count-threshold`, `--restart-window`), `cert_expiry`, `patch_status`, `scheduled_task_failure` (`--scheduled-task-id`, `--restart-count-threshold`), `node_disk_space`, `node_resource_usage`, `node_offline`, `domain_health`, `control_plane_backup_stale`, `log_archive_stale`, `backup_missing` (`--backup-resource-kind database` with `--backup-database-name`, or `volume` with `--backup-volume-name`) and `slo_burn` (`--slo`, optional `--slo-latency-ms`). Mute, schedule and review alerts with the top-level [Alerts](#alerts) commands.

```
levelrail-cli apps connect <app> <database> [--field FIELD] [--env-var NAME] [flags]
levelrail-cli apps disconnect <app> <env-var> [flags]
levelrail-cli apps connections list <app> [flags]
levelrail-cli apps connections suggest <app> [flags]
levelrail-cli apps database set <name> --database-name NAME [--env-var VAR] [--field FIELD] [flags]
levelrail-cli apps database clear <name> [flags]
levelrail-cli apps save-as-template <name> [--template-name NAME] [--description TEXT] [flags]
levelrail-cli apps tag <name> <tag> [flags]
levelrail-cli apps untag <name> <tag> [flags]
```

- `apps connect` connects an app to a managed database and injects its resolved connection value as an env var; unlike `apps database`, an app can have any number of these. `connections list` shows each connection, including whether it resolves to a mesh DNS name (cross node capable) or a container name; `connections suggest` lists databases the app could connect to. `apps disconnect` removes one connection by its env var name.
- `apps database set` attaches one already created managed database as the app's connection env var source (default `DATABASE_URL`, field `url`); `clear` detaches it.
- `apps save-as-template` derives a compose.yaml from the app's current desired state and saves it as a reusable template. No secret, database or vault backed env value is captured, only the key name.
- `apps tag` attaches a tag by name, creating it if new; `apps untag` detaches it. See [Tags](#tags).

### Projects, environments and organizations

```
levelrail-cli apps projects create --name NAME [flags]
levelrail-cli apps projects list [flags]
levelrail-cli apps projects get <id> [flags]
levelrail-cli apps projects delete <id> [--cascade] [flags]
levelrail-cli apps projects start <id> [flags]
levelrail-cli apps projects stop <id> [flags]
levelrail-cli apps projects restart <id> [flags]
levelrail-cli apps projects env-get <id> [flags]
levelrail-cli apps projects env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]
levelrail-cli apps projects topology <id> [flags]
levelrail-cli apps environments create <project-id> --name NAME [--protected] [flags]
levelrail-cli apps environments list <project-id> [flags]
levelrail-cli apps environments update <id> --protected=true|false [flags]
levelrail-cli apps environments delete <id> [--cascade] [flags]
levelrail-cli apps environments env-get <id> [flags]
levelrail-cli apps environments env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]
levelrail-cli apps environments env-diff <project-id> <env-a> <env-b> [flags]
levelrail-cli apps environments clone-preview <id> --new-name NAME [flags]
levelrail-cli apps environments clone <id> --new-name NAME [--app-rename SOURCE=NEWNAME ...] [--domain SOURCE=D1,D2 ...] [--copy-secret-values] [flags]
levelrail-cli apps organizations create --name NAME [flags]
levelrail-cli apps organizations list [flags]
levelrail-cli apps organizations get <id> [flags]
levelrail-cli apps organizations delete <id> [--cascade] [flags]
levelrail-cli apps organizations set-project <project-id> <org-id> [flags]
levelrail-cli apps organizations clear-project <project-id> [flags]
levelrail-cli apps organizations env-get <id> [flags]
levelrail-cli apps organizations env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]
levelrail-cli apps set-project <name> <project-id> [flags]
levelrail-cli apps clear-project <name> [flags]
levelrail-cli apps set-environment <name> <environment-id> [flags]
levelrail-cli apps clear-environment <name> [flags]
```

- A project groups apps and databases; an organization groups projects. Deleting either leaves its members running, simply ungrouped again.
- Shared env vars layer in this order: organization, then project, then environment, then the app's own env, each overriding the one before. `env-set` replaces that scope's shared env vars. For per scope secrets see [Shared env](#shared-env).
- A protected environment requires `--confirm` (or an interactive yes) on `apps deploy`, `apps rollback` and `apps promote` targeting an app tagged with it, and routes the deploy through [deploy approvals](#deploy-history-approvals-and-automation).
- `apps environments clone-preview` shows what cloning would create without applying it; `env-diff` diffs two environments' resolved effective env vars.
- `apps projects topology` prints a diagram ready graph of a project's apps, databases and volumes. See [Projects and organizations](projects-and-organizations.md).

## Tags

```
levelrail-cli tags list [flags]
levelrail-cli tags create --name NAME [flags]
levelrail-cli tags delete <name> [flags]
levelrail-cli tags apps <name> [flags]
```
Tags are arbitrary labels for organizing and filtering apps, always identified by name. `delete` detaches the tag from every app. `apps` lists every app attached to a tag. Attach one to an app with `apps tag`.

## Pipelines

```
levelrail-cli pipelines list <app> [flags]
levelrail-cli pipelines validate <file> [--json]
levelrail-cli pipelines save <app> <file-or-repo-dir> [--name N] [--paths G,...] [--paths-ignore G,...] [--report-status=false] [flags]
levelrail-cli pipelines delete <app> <name> [flags]
levelrail-cli pipelines run <app> <name> [--ref R] [--sha S] [--input k=v]... [--follow] [flags]
levelrail-cli pipelines runs <app> [<run-id>] [--pipeline N] [--limit N] [flags]
levelrail-cli pipelines runs --all [--status S] [--app A] [--trigger T] [--pipeline N] [--limit N] [flags]
levelrail-cli pipelines logs <app> <run-id> [--job KEY] [--follow] [flags]
levelrail-cli pipelines cancel <app> <run-id> [flags]
levelrail-cli pipelines approve <app> <run-id> [--reject] [--comment TEXT] [--approval ID] [flags]
levelrail-cli pipelines sync <app> [--repo-truth=true|false] [flags]
levelrail-cli pipelines triggers <app> [flags]
levelrail-cli pipelines oidc [flags]
levelrail-cli pipelines oidc rotate-key [--retire-after D] [flags]
```

- `validate` checks a pipeline file locally with no API call and exits 2 when it has problems.
- `save` creates or updates pipelines from one file, or from every file in a repository's pipeline directory.
- `runs` lists runs, or shows one run's jobs, steps and approval gates; `--all` lists runs across every app.
- `approve` decides approval gates, or releases a run held for approval.
- `sync` syncs pipeline files from the repository now, or sets the repository as source of truth.
- `triggers` shows why recent git events did or did not start runs.
- `oidc` shows whether pipeline jobs can mint OIDC tokens and the JWKS URL to wire to a cloud provider; `oidc rotate-key` rotates the signing key, and the old key stays published until it retires.

See [Pipelines](pipelines.md) and [Pipeline OIDC](pipelines-oidc.md).

## Load balancers {#lb}

Experimental: requires `APP_EXPERIMENTAL=load-balancer`. See [Load balancing](load-balancing.md).

```
levelrail-cli lb list [--state balancing|degraded|none] [--search Q] [flags]
levelrail-cli lb show <app> [flags]
levelrail-cli lb set <app> [--algorithm ...] [flags]
levelrail-cli lb clear <app> [flags]
levelrail-cli lb status <app> [flags]
levelrail-cli lb check <app> [flags]
levelrail-cli lb history <app> [--limit N] [flags]
levelrail-cli lb upstream <app> <id> --state S [flags]
levelrail-cli lb export <app> --format terraform|cdk|cloudformation|caddy|caddy-json [--out FILE]
levelrail-cli lb import <app> --file app.yaml [--service S] [flags]
```

- `list` shows every load balancer across apps with upstream health. `set` creates or changes the load balancer and only the flags you pass change; `clear` removes it, back to a single upstream.
- `status` is the live upstream table; `check` probes every upstream once, right now; `history` shows recent checks and state changes per upstream; `upstream` sets an upstream active, draining or disabled.
- `export` generates an infrastructure as code definition with no cloud API calls; `import` loads the `loadbalancer:` block of an app.yaml.

## Preview

Deploy previews are the screenshot or metadata card captured after a deploy, not pull request previews (those are `apps previews`). See [Deploy previews](deploy-previews.md).

```
levelrail-cli preview status <app> [flags]
levelrail-cli preview enable <app> [--mode metadata|screenshot] [--path /] [--wait-ms N] [flags]
levelrail-cli preview disable <app> [flags]
levelrail-cli preview capture <app> [flags]
levelrail-cli preview prune <app> [--all] [flags]
```

`enable` turns deploy previews on: `metadata` reads the page title and social image with one small request (no browser), `screenshot` (the default here) runs a browser container per deploy and pulls a large image once. `capture` recaptures the current release now; `prune` deletes old previews now, or every preview of the app with `--all`. Previews need the server to run with `APP_PREVIEW_ENABLED=true` (the default).

## Databases

```
levelrail-cli databases create --name NAME --engine ENGINE --version VERSION [flags]
levelrail-cli databases create --interactive
levelrail-cli databases list [flags]
levelrail-cli databases get <name> [flags]
levelrail-cli databases status <name> [flags]
levelrail-cli databases delete <name> [--force] [flags]
levelrail-cli databases start <name> [flags]
levelrail-cli databases stop <name> [flags]
levelrail-cli databases metrics <name> --metric NAME [flags]
levelrail-cli databases logs <name> [flags]
levelrail-cli databases slow-queries <name> [flags]
levelrail-cli databases schema <name> [--columns] [flags]
levelrail-cli databases query <name> --sql "select ..." [--explain] [--write --confirm <name>] [flags]
levelrail-cli databases connect --name <n> --engine <e> --host <h> [--password-stdin] [--test] [flags]
levelrail-cli databases adopt --container <c> [--node <id>] [--list] [--password-stdin] [flags]
levelrail-cli databases probe <name> [flags]
levelrail-cli databases resource-recommendation <name> [flags]
levelrail-cli databases set-resources <name> [--memory 512Mi] [--cpu 0.5] [flags]
levelrail-cli databases set-project <name> <project-id> [flags]
levelrail-cli databases clear-project <name> [flags]
levelrail-cli databases set-node <name> <node-id> [flags]
levelrail-cli databases clear-node <name> [flags]
levelrail-cli databases public-access set <name> [--port N] [--bind-address ADDR] [flags]
levelrail-cli databases public-access clear <name> [flags]
levelrail-cli databases set-version <name> <version> [flags]
levelrail-cli databases major-upgrade <name> --version V [flags]
levelrail-cli databases major-upgrades <name> [flags]
levelrail-cli databases major-upgrade-rollback <name> <id> [flags]
levelrail-cli databases major-upgrade-discard <name> <id> [flags]
```

- `status` shows a database's current reconcile conditions (useful when it exists but is not running yet).
- `delete` stops and removes a database but keeps its data volume; `--force` is needed if apps use it.
- `slow-queries` lists a Postgres or MySQL database's slow query log.
- `schema` lists a SQL database's tables with row estimates and sizes; `query` runs one statement through the control plane, read-only by default. See [Database viewer](database-viewer.md). `db` is a short alias for `databases`.
- `set-resources` applies memory and CPU limits to an already created database, replacing whatever was set before (a full replace, not a patch).
- `public-access set` exposes a database on a host port; `--bind-address` is `private` (the default), `public`, or a literal IP.
- `set-version` is a minor or patch image change on the same data. `major-upgrade` is a guarded Postgres major upgrade with a rollback snapshot; `major-upgrades` lists the attempts, `major-upgrade-rollback` restores the pre upgrade data, and `major-upgrade-discard` deletes a rollback snapshot to free disk.

See [Managing databases](managing-databases.md).

## AI models {#models}

Experimental: requires `APP_EXPERIMENTAL=ai-models`. See [AI models](ai-models.md).

```
levelrail-cli models list [flags]
levelrail-cli models get <name> [flags]
levelrail-cli models deploy --name NAME --engine ENGINE --model MODEL [flags]
levelrail-cli models logs <name> [flags]
levelrail-cli models delete <name> [flags]
levelrail-cli models restart <name> [flags]
levelrail-cli models rotate-key <name> [flags]
levelrail-cli models keys list|create|revoke|rotate [flags]
levelrail-cli models usage <name> [flags]
levelrail-cli models residency <name> --mode M [flags]
levelrail-cli models swap-group <name> --group G [flags]
levelrail-cli models wake|sleep <name> [flags]
levelrail-cli models fit --engine E --model M [flags]
levelrail-cli models metrics <name> [flags]
levelrail-cli models gpus [flags]
levelrail-cli models preflight <repo> [flags]
levelrail-cli models cache list [flags]
levelrail-cli models cache prune [--dry-run] [volume...] [flags]
```

- `deploy` runs a model as a container on a GPU node and prints its API key once. Flags: `--engine` (`ollama`, `vllm` or `llamacpp`), `--model`, `--node`, `--gpus` (a count or `all`), `--gpu-devices`, `--context`, `--quantization`, `--domain`, `--hf-token-from-env`, `--residency` (`always` or `on_demand`), `--idle-ttl`, `--swap-group`.
- `logs` searches a model engine's logs, or `--follow` streams download and load progress live. `delete` keeps the downloaded weights volume. `restart` recreates the engine container.
- `rotate-key` issues a new API key, printed once; `keys` manages named API keys with limits, expiry and rotation grace; `usage` shows gateway requests, tokens, errors and latency per key.
- `residency` sets a model always resident or `on_demand` (stopped when idle, started on first request); `wake` and `sleep` start or stop an on demand engine now; `swap-group` lets models share a GPU (`--clear` removes it).
- `fit` estimates VRAM fit on each GPU node; `gpus` lists GPU nodes with VRAM and usage; `preflight` checks a Hugging Face repo (access, size, quantizations, fit, disk); `metrics` shows engine metrics.
- `cache list` shows cached model weights per node; `cache prune` removes unused, unreferenced weights (use `--dry-run` first).

## Auth

```
levelrail-cli auth login [--device] [--profile NAME] [flags]
levelrail-cli auth whoami [flags]
levelrail-cli auth session-link [flags]
levelrail-cli auth code [flags]
levelrail-cli auth approve <id> --match N [flags]
levelrail-cli auth deny <id> [flags]
levelrail-cli auth devices [revoke <id>] [flags]
levelrail-cli auth code-login [--admins true|false] [--others true|false] [flags]
levelrail-cli auth 2fa status [flags]
levelrail-cli auth 2fa setup [flags]
levelrail-cli auth 2fa enable --code CODE [flags]
levelrail-cli auth 2fa disable --code CODE|--recovery-code CODE [flags]
levelrail-cli auth 2fa recovery-codes --code CODE [flags]
```

- `login` authenticates and persists a new API token; with `--profile NAME` it saves under a named profile instead of overwriting `default`.
- `login --device --json` prints one JSON line first (`verification_url`, `user_code`, `expires_in`, `expires_at`) so an agent can relay it, then the token resource once approved. See [Attention center](attention-center.md).
- `session-link` mints a short lived (about 2 minutes), single use login link that signs in as whoever minted it, meant for browser automation. It needs a token with the root ability.
- `code` prints the sign-in codes waiting for your account with the requester's IP address, browser and time, plus password sign-ins from new browsers waiting for approval; `approve` and `deny` decide one of those (`approve` needs `--match` with the two digit number the waiting browser shows, typed in; a wrong number denies it, and the number is never printed by `code`), and `devices` lists or revokes browsers trusted for password sign-in. They need a token that belongs to your user. Listing and `devices` accept `write:sensitive`; showing a code and `approve` or `deny` need `signin:approve`, which no device login, root or `write:sensitive` token carries. Mint one from a signed-in session with `levelrail-cli tokens create --name approver --abilities signin:approve --expires-in-days 30` (it must expire within 30 days by default, cannot be an agent token, and is revoked when you change or reset your password or sign out your other sessions), or use the dashboard. `code-login` shows or changes who may sign in with a code (changing it needs root). See [Identity and access](identity-and-access.md#sign-in-with-a-code).
- Every `2fa` subcommand needs a live session: `--username` and `--password` (prompted if omitted), never the CLI's saved bearer token. `setup` starts enrollment and returns a secret and provisioning URI; `enable` confirms it and returns recovery codes once; `recovery-codes` regenerates them and invalidates the old set.

## Auth engine

```
levelrail-cli auth-engine status [flags]
```
Shows which auth engine is active, the areas served by the library, and the shadow comparison counters with the most recent mismatches. Needs a root token.

## Profile

```
levelrail-cli profile list [flags]
```
List configured credentials profiles and their API URLs.

## Tokens

```
levelrail-cli tokens create --name NAME (--abilities LIST | --preset observer|deployer|operator) [--agent NAME] [--agent-description TEXT] [flags]
levelrail-cli tokens list [flags]
levelrail-cli tokens revoke <id> [flags]
```
Every `tokens` subcommand requires a username and password (prompted if omitted), because these routes are session only server side and a bearer token cannot call them. `create` mints a new API token; `--agent` labels it as issued to an AI agent so audit entries record the agent name. `list` never shows a secret.

## Domains

Per domain settings take `<app> <domain>`, and the domain must already be one of the app's configured domains. Domain TLS and ingress behavior is covered in [Domains and ingress](domains-and-ingress.md).

```
levelrail-cli domains list [flags]
levelrail-cli domains check <app> <domain> [flags]
levelrail-cli domains doctor <app> <domain> [--json]
levelrail-cli domains summary [--json]
levelrail-cli domains activity <domain> [--limit N] [--before CURSOR] [--actions PREFIXES] [--json]
levelrail-cli domains certificates [flags]
levelrail-cli domains basic-auth get|set|clear <app> <domain> [flags]
levelrail-cli domains maintenance get|set|clear <app> <domain> [flags]
levelrail-cli domains redirect get|set|clear <app> <domain> [--target URL] [flags]
levelrail-cli domains tls-cert get <app> <domain> [flags]
levelrail-cli domains tls-cert set <app> <domain> --cert-file FILE --key-file FILE [flags]
levelrail-cli domains tls-cert clear <app> <domain> [flags]
levelrail-cli domains tls-cert renew <app> <domain> [flags]
levelrail-cli domains waf get|set|clear <app> <domain> [flags]
levelrail-cli domains error-pages get <app> <domain> [--code N] [flags]
levelrail-cli domains error-pages set <app> <domain> --code N (--body TEXT | --body-file PATH) [flags]
levelrail-cli domains error-pages clear <app> <domain> [--code N] [flags]
levelrail-cli domains dns list <app> <domain> [flags]
levelrail-cli domains dns add <app> <domain> --type T --name N --value V [flags]
levelrail-cli domains dns remove <app> <domain> --type T --name N --value V [flags]
levelrail-cli domains go-live <app> <domain> [--dns auto|off|preview] [--replace] [--plan] [--wait] [--timeout 2m] [flags]
levelrail-cli domains go-live runs [--limit N] [flags]
levelrail-cli domains go-live undo <run-id> [flags]
levelrail-cli domains backfill-base-domain [--confirm] [flags]
levelrail-cli settings domain-automation get|set [flags]
levelrail-cli domains cloudflare-dns get|set|clear [flags]
levelrail-cli domains route53-dns get|set|clear [flags]
```

- `list` shows every app's domains in one call. `check` runs a real DNS lookup and reports whether the domain reaches this control plane. `certificates` lists every certificate in certmagic storage, healthy or not.
- `basic-auth set` takes `--username` and `--password`. `maintenance set` serves a fixed maintenance response instead of proxying, without stopping the container. `redirect set --target URL` redirects every request for the domain; maintenance mode takes precedence if both are set.
- `tls-cert` uploads an operator supplied certificate and key; `clear` reverts to automatic issuance and `renew` forces re-issuance of an automatic certificate.
- `waf` configures the opt-in Web Application Firewall and rate limiting. `--mode` defaults to `detect` (matches are logged, never rejected) rather than `block`.
- `error-pages` sets custom HTML for status code 404, 500, 502 or 503; `clear` removes one mapping, or all if `--code` is omitted.
- `dns` reads and writes the real A, AAAA, CNAME, TXT, MX, SRV and CAA records in the domain's zone through whichever DNS-01 provider is configured. `remove` must match an existing record exactly.
- `cloudflare-dns set --cf-api-token TOKEN` and `route53-dns set --aws-access-key-id ID --aws-secret-access-key KEY` configure the ACME DNS-01 credentials needed for wildcard domains. A domain is wildcard eligible by having a leading `*.` label; if both providers are enabled, Cloudflare takes precedence.

## DNS

See [DNS zones and records](dns.md).

```
levelrail-cli dns zones list [--provider cloudflare|route53]
levelrail-cli dns zones create <domain> [--account-id ID]
levelrail-cli dns zones nameservers <zone>
levelrail-cli dns zones verify <zone>
levelrail-cli dns zones delete <zone> --confirm <zone> [--force]
levelrail-cli dns records list <zone> [--type T] [--search S]
levelrail-cli dns records add|update <zone> --name N --type T --value V [--value V2] [--ttl S] [--proxied] [--routing weighted|failover|multivalue --set-id ID ...]
levelrail-cli dns records delete <zone> --name N --type T [--set-id ID]
levelrail-cli dns records import <zone> --file F [--format bind|json] [--apply] [--replace --confirm <zone>]
levelrail-cli dns records export <zone> [--format bind|json] [--out F]
levelrail-cli dns records template <zone> <template> [--param KEY=VALUE]... [--apply]
levelrail-cli dns check <name> [--type T] [--zone Z]
levelrail-cli dns health-checks list|create|delete
```

- `zones verify` compares the NS set the system resolver, 1.1.1.1 and 8.8.8.8 return with the zone's assigned name servers.
- `records import` and `records template` print a plan and change nothing until `--apply`. `--replace` deletes sets missing from the file and needs `--confirm`.

## Backups

See [Backups and storage](backups-and-storage.md).

```
levelrail-cli backups list <database> [flags]
levelrail-cli backups list-all [flags]
levelrail-cli backups trigger <database> --target ID [flags]
levelrail-cli backups delete <database> <backup-id> [flags]
levelrail-cli backups download <database> <backup-id> [flags]
levelrail-cli backups restore <database> --backup ID --confirm NAME [flags]
levelrail-cli backups restore-as-new <database> --backup ID --new-name NAME [flags]
levelrail-cli backups restores <database> [flags]
levelrail-cli backups clone-restores <database> [flags]
levelrail-cli backups schedule set <database> --target ID --cron EXPR [flags]
levelrail-cli backups schedule clear <database> [flags]
levelrail-cli backups verify <database> --backup ID [flags]
levelrail-cli backups verifications <database> --backup ID [flags]
```

- `list-all` lists backup history across every database and app volume instance wide.
- `delete` removes one archived backup (destructive). `download` streams a succeeded backup's object to stdout.
- `restore` restores in place and is destructive; `--confirm` takes the database name. `restore-as-new` restores into a brand new database and is non destructive. `restores` and `clone-restores` list the attempt history of each.
- `verify` checks a backup is intact without a live restore; `verifications` lists past attempts.

## App volume backups

```
levelrail-cli app-volume-backups list <app> <volume> [flags]
levelrail-cli app-volume-backups trigger <app> <volume> --target ID [flags]
levelrail-cli app-volume-backups delete <app> <volume> <backup-id> [flags]
levelrail-cli app-volume-backups download <app> <volume> <backup-id> [flags]
levelrail-cli app-volume-backups restore <app> <volume> --backup ID --confirm APP/VOLUME [flags]
levelrail-cli app-volume-backups restore-as-new <app> <volume> --backup ID [--new-volume-name NAME] [flags]
levelrail-cli app-volume-backups restores <app> <volume> [flags]
levelrail-cli app-volume-backups clone-restores <app> <volume> [flags]
levelrail-cli app-volume-backups schedule set <app> <volume> --target ID --cron EXPR [flags]
levelrail-cli app-volume-backups schedule clear <app> <volume> [flags]
levelrail-cli app-volume-backups verify <app> <volume> --backup ID [flags]
levelrail-cli app-volume-backups verifications <app> <volume> --backup ID [flags]
```

The same verbs as `backups`, for a named volume declared in the app's app.yaml (`apps volumes get <app>` lists them). `restore` is destructive and its `--confirm` takes `APP/VOLUME`; `restore-as-new` creates a standalone volume.

## Point-in-time restore {#pitr-restores}

Postgres only. Recovery covers the time range from when `pitr enable` was called, forward; use `backups restore` for anything older.

```
levelrail-cli pitr enable <database> [flags]
levelrail-cli pitr disable <database> [flags]
levelrail-cli pitr status <database> [flags]
levelrail-cli pitr base-backups list <database> [flags]
levelrail-cli pitr base-backups trigger <database> --target ID [flags]
levelrail-cli pitr restores <database> [flags]
levelrail-cli pitr restore <database> --base-backup ID --target-time RFC3339 [--confirm NAME] [flags]
```

`enable` turns on continuous WAL archiving going forward; `status` shows whether PITR is enabled and the current recoverable window; `restores` lists point in time restore attempts (base backup, target time, status, error). `restore` is destructive.

## Build

```
levelrail-cli build detect --repo-url URL [--ref REF] [flags]
levelrail-cli build branches --repo-url URL [flags]
```
Both work against a public repository and need no existing app. `detect` shows which framework the builder detects without running a build, and prints `no framework detected` (exit 0) when nothing matches. `branches` lists the branches a repo advertises; private or unreachable repos fail with an API error.

## Cloudflare Tunnel

Experimental: requires `APP_EXPERIMENTAL=cloudflare-tunnel`.

```
levelrail-cli cloudflare-tunnel get [flags]
levelrail-cli cloudflare-tunnel set --cf-tunnel-token TOKEN [flags]
levelrail-cli cloudflare-tunnel disconnect [flags]
```
Runs the `cloudflared` container with a tunnel token from your own Cloudflare Zero Trust dashboard, exposing the control plane without an inbound port. This is a different credential from `domains cloudflare-dns`.

## Vault

```
levelrail-cli vault get [flags]
levelrail-cli vault set --address URL --auth-method token --vault-token TOKEN [flags]
levelrail-cli vault set --address URL --auth-method approle --role-id ID --secret-id SECRET_ID [flags]
levelrail-cli vault disconnect [flags]
```
Configures resolving app secrets live from an external HashiCorp Vault, as an alternative to the platform's own envelope encrypted storage. The credential is stored encrypted and never shown back; `role_id` is echoed on `get`. `disconnect` disables and forgets the stored credential.

## Channels

```
levelrail-cli channels list [flags]
levelrail-cli channels create --name NAME --kind KIND --notify-url URL [flags]
levelrail-cli channels update <id> --name NAME --kind KIND --notify-url URL [flags]
levelrail-cli channels delete <id> [flags]
levelrail-cli channels test <id> [flags]
levelrail-cli channels deliveries <id> [flags]
```
Valid `--kind` values: `generic`, `slack`, `discord`, `telegram`, `email`, `pushover`, `pagerduty`, `teams`, `resend`, `ntfy`, `gotify`, `mattermost`, `lark`, `rocketchat`, `opsgenie`, `webex`, `googlechat`. `update` fully replaces a channel's configuration. `--notify-device-login` opts a channel into a link-only notice when a CLI login is waiting. `test` sends a real test message; `deliveries` lists a channel's recorded send history, newest first. See [Email notifications](email-notifications.md).

## Push subscriptions

```
levelrail-cli push-subscriptions list [flags]
levelrail-cli push-subscriptions revoke <id> [flags]
```
Lists or revokes the browser push subscriptions registered for the caller's own account. There is no `register` subcommand: a subscription's endpoint and keys only come from a real browser. See [Observability](observability.md).

```
levelrail-cli push-subscriptions list --json
levelrail-cli push-subscriptions revoke <id>
```

## Alerts

Top-level commands for muting, scheduling and reviewing alerts. The rules themselves are managed under [`apps alerts`](#alerts-connections-and-templates). See [Observability](observability.md).

```
levelrail-cli alerts silences list [--all]
levelrail-cli alerts silences create --for DURATION [--rule ID] [--app NAME] [--node NAME] [--kind KIND] [--severity S] [--label k=v] [--reason TEXT]
levelrail-cli alerts silences delete <id>
levelrail-cli alerts silence <app> <rule-id> [--for 1h] [--reason TEXT]
levelrail-cli alerts maintenance list
levelrail-cli alerts maintenance create --name NAME --cron "0 3 * * 0" --duration 2h [--tz Europe/Berlin] [--scope all|app|node] [--target NAME]...
levelrail-cli alerts maintenance update <id> [same flags as create] [--disabled]
levelrail-cli alerts maintenance delete <id>
levelrail-cli alerts history [--app NAME] [--rule ID] [--outcome OUTCOME] [--event EVENT] [--since TIME] [--limit N] [--changes]
```

- A silence keeps matching alerts evaluating and recorded in history but stops them notifying. Every filter given must match; repeat a flag to accept any of several values. `silences delete` ends a silence now and keeps it in history (`silences list --all`).
- `silence` is the quick action: silence one rule, `--for` defaults to `1h`.
- A maintenance window is a recurring silence: a 5 field cron expression for each start, a `--duration`, and `--tz` (default `UTC`). `--scope` is `all` (default), `app` or `node`, with `--target` naming the apps or nodes.
- `history` lists firings and resolutions, newest first (`--limit` defaults to 50). `--outcome` is one of `sent`, `silenced`, `grouped`, `inhibited`, `failed`, `ratelimited`, `flapping` or `skipped`; `--event` is `fired`, `resolved`, `flapping` or `flap_ended`; `--since` takes an RFC 3339 timestamp. `--changes` adds what changed on the app before each firing, with the most likely cause tagged.

```
levelrail-cli alerts silence web <rule-id> --for 4h --reason "planned migration"
levelrail-cli alerts history --app web --outcome failed --limit 20
```

## Status page

The opt-in public status page. It stays off until `set --enable`. Only public component names, statuses and operator written announcements are published; targets (app names, hostnames, check URLs) stay private. See [Status page](status-page.md).

```
levelrail-cli status-page get
levelrail-cli status-page set [--enable|--disable] [--title T] [--description D] [--domain HOST]
levelrail-cli status-page preview
levelrail-cli status-page components list
levelrail-cli status-page components add --kind app|domain|check --target T --name PUBLIC_NAME
levelrail-cli status-page components delete <id>
levelrail-cli status-page incidents list
levelrail-cli status-page incidents create --title T [--kind incident|maintenance] [--impact none|minor|major|critical] [--body TEXT] [--component ID]... [--starts TIME --ends TIME]
levelrail-cli status-page incidents update <id> --status S --body TEXT
levelrail-cli status-page incidents delete <id>
```

- `--enable` and `--disable` are mutually exclusive. `--domain` serves the page on a hostname that the ingress must route to the control plane.
- `incidents create`: `--kind` defaults to `incident`, `--impact` to `none`. A `maintenance` kind requires `--starts` and `--ends` (RFC 3339). `--body` is markdown-lite.
- `incidents update --status` takes `investigating`, `identified`, `monitoring` or `resolved` for an incident, and `scheduled`, `in_progress` or `completed` for maintenance.

```
levelrail-cli status-page set --enable --title "Example status"
levelrail-cli status-page components add --kind app --target web --name "Website"
levelrail-cli status-page incidents create --title "Elevated errors" --impact minor --body "We are investigating."
```

## Shared env

Environment variables shared at project, organization or environment scope, plain or secret. See [Projects and organizations](projects-and-organizations.md).

```
levelrail-cli shared-env list --scope SCOPE --id ID [flags]
levelrail-cli shared-env set --scope SCOPE --id ID <key> <value> [--secret] [flags]
levelrail-cli shared-env delete --scope SCOPE --id ID <key> [--secret] [flags]
```

`SCOPE` is `project`, `organization` or `environment`, and `--id` is that resource's id. A plain value is stored as is. `--secret` encrypts it the same way an app's `{ secret: true }` env vars are, and its value is never shown again, only its key. `list` shows plain and secret marked entries alike; pass `--secret` to `delete` when the key is secret marked.

```
levelrail-cli shared-env set --scope project --id proj_abc123 LOG_LEVEL info
levelrail-cli shared-env set --scope project --id proj_abc123 DB_PASSWORD "$DB_PASSWORD" --secret
```

## Registry

```
levelrail-cli registry status [flags]
levelrail-cli registry enable --host HOST [flags]
levelrail-cli registry disable [flags]
levelrail-cli registry repositories [flags]
levelrail-cli registry tags --repository NAME [flags]
```
Runs the platform's own built-in image registry (`registry:2`). The first `enable` generates a username and password; the password is printed once and never shown again. `disable` forgets the generated credentials. This is distinct from `registry-credentials`, which stores credentials for an external registry.

## Registry credentials

```
levelrail-cli registry-credentials list [flags]
levelrail-cli registry-credentials get <id> [flags]
levelrail-cli registry-credentials create --name NAME --registry-host HOST --username USER --password PASS [flags]
levelrail-cli registry-credentials update <id> --name NAME --registry-host HOST --username USER [flags]
levelrail-cli registry-credentials delete <id> [flags]
levelrail-cli registry-credentials test <id> [flags]
levelrail-cli registry-credentials repositories <id> [flags]
levelrail-cli registry-credentials tags <id> <repository> [flags]
```
Private container registry pull credentials. `update` can rotate the password. `test` authenticates against the registry without pulling anything; `repositories` and `tags` browse the external registry.

## Backup targets

```
levelrail-cli backup-targets list [flags]
levelrail-cli backup-targets get <id> [flags]
levelrail-cli backup-targets create --name NAME --provider PROVIDER --bucket BUCKET --access-key-id ID --secret-access-key KEY [flags]
levelrail-cli backup-targets update <id> --name NAME --provider PROVIDER --bucket BUCKET [flags]
levelrail-cli backup-targets delete <id> [flags]
levelrail-cli backup-targets test <id> [flags]
```
S3 compatible backup destinations. Valid `--provider` values are `aws`, `r2` and `custom`; `--endpoint` is required for `r2` and `custom`. `test` probes the bucket over its stored credentials without uploading or deleting anything.

## Storage

```
levelrail-cli storage providers
levelrail-cli storage list
levelrail-cli storage add --name N --provider P --bucket B --access-key-id ID --secret-access-key KEY [flags]
levelrail-cli storage test <id>
levelrail-cli storage delete <id>
```
S3 compatible storage destinations. `providers` lists the presets (`aws`, `r2`, `b2`, `minio`, `wasabi`, `custom`). `add` also takes `--account-id` (r2), `--region` (aws, b2, wasabi), `--endpoint` (minio, custom), `--virtual-hosted` and `--skip-verify`. `test` writes, reads back and deletes a probe object.

## Logs

See [Log archive](log-archive.md).

```
levelrail-cli logs archive set --target ID [--app NAME] [--interval 1h] [--retention-days N] [--disable]
levelrail-cli logs archive status
levelrail-cli logs archive remove [--app NAME]
levelrail-cli logs dump --target ID --from TIME [--to TIME] [--app NAME] [--wait]
levelrail-cli logs ls --target ID [--app NAME]
levelrail-cli logs fetch --target ID --key KEY [--out FILE]
levelrail-cli logs query <app> [--level LEVEL] [--since 30m] [--until T] [--deploy ID] [--text PHRASE] [--max-lines N] [--max-bytes N] [flags]
```
`TIME` is an RFC 3339 timestamp or a duration back from now such as `24h`. Without `--app`, a policy or dump covers every app. `fetch` downloads one archived object (gzip NDJSON). `query` returns a capped excerpt of an app's newest matching log lines with match counts and a truncation notice; the byte cap defaults to 8 KB or `APP_MCP_LOG_MAX_BYTES`.

## Network shares

NFS and CIFS network share mounts, the same shares an app's volume mount can reference by name. See [Network shares](network-shares.md).

```
levelrail-cli network-shares list [flags]
levelrail-cli network-shares get <id> [flags]
levelrail-cli network-shares create --name NAME --protocol nfs|cifs --host HOST --remote-path PATH [--mount-options OPTS] [--username USER --password PASS] [flags]
levelrail-cli network-shares update <id> --name NAME --protocol nfs|cifs --host HOST --remote-path PATH [flags]
levelrail-cli network-shares delete <id> [flags]
levelrail-cli network-shares test <id> [flags]
```

`--name`, `--protocol`, `--host` and `--remote-path` are required on `create` and `update`. A CIFS share also needs `--username` and `--password`; on `update`, omit `--password` to keep the existing one. `test` checks that the share's host is reachable on its protocol's standard port.

```
levelrail-cli network-shares create --name media --protocol nfs --host nas.internal --remote-path /exports/media
levelrail-cli network-shares test <id>
```

## Firewall

Declarative host firewall rules, reconciled onto the control plane's local `ufw`. See [Host firewall](host-firewall.md). `exposure`, `restrict` and `unrestrict` are covered in [Exposure audit](exposure-audit.md).

```
levelrail-cli firewall status [flags]
levelrail-cli firewall enable [--dry-run] [flags]
levelrail-cli firewall disable [--dry-run] [flags]
levelrail-cli firewall exposure [--node N] [--probe] [flags]
levelrail-cli firewall restrict --port N --allow CIDR[,CIDR] [--local-containers] --dry-run|--apply [flags]
levelrail-cli firewall unrestrict --port N [--protocol tcp|udp] [flags]
levelrail-cli firewall list [flags]
levelrail-cli firewall allow --port N [--protocol tcp|udp] [--source-cidr CIDR] [--label TEXT] [flags]
levelrail-cli firewall deny --port N [--protocol tcp|udp] [--source-cidr CIDR] [--label TEXT] [flags]
levelrail-cli firewall delete <id> [flags]
```

`--port` is required (1 to 65535); `--protocol` defaults to `tcp` and `--source-cidr` to any source. A deny rule, or an allow rule scoped to a source CIDR, that targets a port the control plane itself needs (the management API, agent connections or ingress) is refused rather than applied.

```
levelrail-cli firewall allow --port 5432 --source-cidr 10.0.0.0/8 --label "postgres from LAN"
levelrail-cli firewall list
```

## Feature flags

```
levelrail-cli flags create <app> --key KEY --name NAME [--description DESC] [--disabled] [--rollout PERCENT] [flags]
levelrail-cli flags list <app> [flags]
levelrail-cli flags get <app> <id> [flags]
levelrail-cli flags set <app> <id> --name NAME [--description DESC] [--disabled] [--rollout PERCENT] [flags]
levelrail-cli flags delete <app> <id> [flags]
```
A boolean, plus an optional gradual rollout percentage, that the app's own code reads live through `GET /api/v1/flags/evaluate/{key}` with a read scoped API token. Changes take effect immediately with no redeploy. The key is globally unique across the control plane and cannot change after create.

## Apply, diff and export {#apply-diff-and-export}

Experimental: requires `APP_EXPERIMENTAL=iac`. See [Platform as code](platform-as-code.md) for the document format, secrets handling, prune rules and CI use.

```
levelrail-cli apply -f file|dir|- [--dry-run] [--exit-code] [--prune --source NAME] [--project P] [--yes] [--secret K=env:VAR] [--var NAME=VALUE] [--var-file PATH] [--allow-env NAME[,NAME...]] [--no-deploy] [--continue-on-error] [flags]
```
Validates resource files, prints the plan, and applies it through the API with your own permissions. Exit 0 no changes or applied, 1 error, 2 changes pending (with `--dry-run --exit-code`). <span v-pre>`${{ env.NAME }}`</span> placeholders are filled only from `--var`, `--var-file` or the names listed with `--allow-env` (a trailing `*` allows a prefix, but never covers credential looking names such as `AWS_*`, `GITHUB_TOKEN` or anything containing `TOKEN`, `SECRET`, `PASSW` or `_KEY`, which must be named exactly). An unresolved placeholder fails before anything is sent.

```
levelrail-cli diff -f dir [flags]
```
Drift between the files and live state; exits 2 when they differ.

```
levelrail-cli export [--project P] [--app A] [-o dir|-] [--include-env-values=false] [flags]
```
Writes live state as stable resource files, never containing secret values. Secret looking values become <span v-pre>`${{ env.NAME }}`</span> placeholders; supply them at apply time with `--var`, `--var-file` or `--allow-env`.

## Nodes

See [Multi-node](multi-node.md).

```
levelrail-cli nodes list [flags]
levelrail-cli nodes get <id> [flags]
levelrail-cli nodes delete <id> [flags]
levelrail-cli nodes join-token [flags]
levelrail-cli nodes reenroll-token <id> [flags]
levelrail-cli nodes revoke-cert <id> [flags]
levelrail-cli nodes cordon <id> [flags]
levelrail-cli nodes uncordon <id> [flags]
levelrail-cli nodes drain <id> [--target NODE-ID] [flags]
levelrail-cli nodes workloads <id> --accepts-app=BOOL --accepts-build=BOOL [flags]
levelrail-cli nodes health <id> [flags]
levelrail-cli nodes patch-status <id> [flags]
levelrail-cli nodes events <id> [--limit N] [flags]
levelrail-cli nodes metrics <id> --metric NAME [flags]
levelrail-cli nodes resource-usage [flags]
levelrail-cli nodes capacity-forecast <id> [flags]
levelrail-cli nodes mesh [flags]
levelrail-cli nodes rotate-key <id> [flags]
levelrail-cli nodes rejoin-mesh <id> [flags]
levelrail-cli nodes topology [flags]
levelrail-cli nodes traffic [flags]
levelrail-cli nodes providers list|set-credential [flags]
levelrail-cli nodes provision --provider P --region R --size S --name NAME [flags]
levelrail-cli nodes provisions list|show <id> [flags]
levelrail-cli nodes ssh-provision --host HOST --user USER (--key-file FILE | --password PASS) --name NAME [flags]
levelrail-cli nodes ssh-provisions list|show <id> [flags]
```

- `delete` fails while the node still has placements; drain it first. `join-token` mints a one-time enrollment token, shown once. `reenroll-token` mints a one-time token that re-issues a node's agent certificate, keeping its identity, and prints the command to run on the node. `revoke-cert` revokes the certificate and closes the node's session; only a re-enroll token brings it back.
- `cordon` marks a node unschedulable without evacuating it; `uncordon` reverses that; `drain` moves every service and database off a node.
- `list` shows each node's certificate state and days left (CERT) and agent version (AGENT); `get` adds expiry, last renewal, key origin, platform and commit.
- `resource-usage` shows every node's latest CPU, memory and disk usage plus a fleet rollup; `capacity-forecast` projects disk and memory usage forward, roughly how many days until full.
- `mesh` shows the live WireGuard mesh state and peers; `rotate-key` rotates a node's WireGuard key; `rejoin-mesh` forces an immediate resync for a stuck peer.
- `providers` manages cloud provider credentials (Hetzner, DigitalOcean) and `provision` creates a server at a provider and enrolls it as a node; `ssh-provision` adopts a machine you already have over SSH. `provisions` and `ssh-provisions` track either through to enrollment. See [Multi-cloud provisioning](multi-cloud-provisioning.md).
- `topology` is a whole mesh summary (nodes, apps, databases, load balancers, connections); `traffic` shows per domain ingress reachability and TLS status.

## Status

```
levelrail-cli status [flags]
```
Shows the control plane's own configured and not configured signals, including whether its local Docker daemon is reachable.

## Upgrade

```
levelrail-cli upgrade [--no-backup] [flags]
```

Compares the running version with the latest release, runs the preflight checks (release signature, Docker Engine, free disk, backup), takes a control plane backup and prints the command that upgrades. It never upgrades by itself. See [Installing](installing.md#check-first-then-upgrade).

## Version

```
levelrail-cli version [flags]
```
Shows the control plane's running version, and whether a newer release has been published to the project's GitHub repository.

## Changelog

```
levelrail-cli changelog [--limit N] [flags]
```
Prints recent release notes, newest first, from the running control plane's own CHANGELOG.md. Without `--limit` the server default applies. The dashboard shows the same notes in the [What's new panel](whats-new-panel.md).

```
levelrail-cli changelog --limit 3
```

## Init

```
levelrail-cli init [--dir DIR] [--mode MODE] [--mcp-binary NAME] [--dry-run] [--force] [--yes] [flags]
```
Detects the project's stack (Dockerfile, compose file, package.json, go.mod, Python, Java, static site) and writes three files into the directory: `app.yaml` (the app spec, validated with the platform's own validator), `AGENTS.md` (how an AI agent deploys, checks and rolls back the app) and `.mcp.json` (MCP server config; the token is an env var reference, never a value).

- Existing files are never overwritten unless `--force` is given. A diff of what would change is shown either way.
- Without `--yes`, a terminal is asked to confirm; a script must pass `--yes` or `--dry-run`.
- `--mode` selects the tool exposure written to `.mcp.json`: `agent-core` (the default, a small profile for deploy and debug work), `read-only` (every read tool, no mutations), `standard` (read and mutating tools, no destructive ones) or `full`.
- `--api-url` or the active profile is written into `AGENTS.md` and `.mcp.json` when known. `--mcp-binary` overrides the MCP server binary name.

```
levelrail-cli init --dry-run
levelrail-cli init --mode read-only --yes
```

See [Agents](agents.md).

## API docs

```
levelrail-cli api-docs [flags]
```
Prints every HTTP route registered on the control plane: method, path, required ability and resource group, followed by a count of routes and how many have a worked request and response example. It is the same data as Settings, API explorer in the dashboard, without the interactive try-it. Supports `--json`, `--output` and `--query`. See [API explorer](api-explorer.md).

```
levelrail-cli api-docs --query "routes[?method=='GET'].path"
```

## AI assistant

Experimental: requires `APP_EXPERIMENTAL=ai-chat`, and a provider, model and key set with `settings ai-assistant set`. The assistant is read and suggest only: a tool call that would change state pauses for an explicit confirmation. See [In-app AI assistant chat](ai-assistant-chat.md).

```
levelrail-cli ai chat "<message>" [--session ID] [flags]
levelrail-cli ai sessions list [flags]
levelrail-cli ai sessions get <id> [flags]
levelrail-cli ai sessions delete <id> [flags]
levelrail-cli ai sessions resolve <session-id> <confirmation-id> --approve|--reject [flags]
```

- `ai chat` sends one message and streams the reply to stdout. Without `--session` it starts a new session and prints its id to stderr; pass that id back with `--session` to continue the conversation. `--json` prints one JSON object per stream event.
- `ai sessions resolve` approves or rejects a mutating tool call the assistant proposed (shown by `ai chat` as a pending confirmation), then streams the continuation.

```
levelrail-cli ai chat "why did the last deploy of web fail?"
levelrail-cli ai sessions resolve <session-id> <confirmation-id> --approve
```

## AI control

Control what AI agents can do on your platform.

```
levelrail-cli ai-control status [flags]
levelrail-cli ai-control set --mode off|observe|operate|admin [--env-kinds K,K,...] [flags]
levelrail-cli ai-control revoke-agents --yes [flags]
```

- `status` shows the current mode, allowed environment kinds, and when it was last changed.
- `set` changes the mode or allowed environment kinds. Agent tokens cannot approve deploy or pipeline approvals, and cannot change this setting.
- `revoke-agents` revokes every labeled agent token.

See [AI control](ai-control.md).

## Audit log

```
levelrail-cli audit-log [flags]
levelrail-cli audit-purge [flags]
```

`audit-log` lists every recorded write, deploy and root tier request plus automatic certificate renewals, newest first. Read only requests are not recorded. It needs an admin or root scoped token. Flags: `--limit N`, `--before RFC3339`, `--path`, `--method`, `--client-kind cli|dashboard|mcp|api`, `--agent <name>` (entries made with a token labeled with that agent name), `--search <text>` (case-insensitive substring across actor, ability, method, path and remote address), `--resource domain:<name>|zone:<name>|app:<name>` (one resource's own trail), `--actions <prefixes>` (comma separated action prefixes such as `dns_record.,proxy_route.,domain.`), `--failed` (status 400 or higher), `--format csv` with `--output-file FILE`. `--agent`, `--search` and `--failed` are applied server side and carry into csv exports.

`audit-purge` deletes every entry older than the retention window (`APP_AUDIT_LOG_RETENTION_DAYS`, default 90 days) right now instead of waiting for the automatic sweep. It needs an admin or root scoped token.

## Attention

```
levelrail-cli attention [flags]
```

Lists everything that needs attention right now: failing apps, failed deploys from the last 24 hours, low disk space (warn under 10 percent free, critical under 5), offline nodes, expired or expiring certificates, and doctor warnings or failures, critical first. It is the CLI side of the dashboard's Status page (`/status`). Exit code is 1 if any item is critical, 0 otherwise, so it works as a script gate. Supports `--json`, `--output json|table|text`, and `--query`.

## Doctor

```
levelrail-cli doctor [flags]
```

Runs a local preflight health check: Docker daemon reachability, disk space and write latency, write access on the data directory, port 80 and 443 availability for the embedded ingress, control plane database reachability, RAM and CPU against the recommended minimums, firewall status, outbound network reachability (public IP, external port reachability, ACME, clock skew, the agent advertise host, image registries), and GPU attach checks. Exit code is 0 if every check is ok or warn, 1 if any check fails.

## Proxy

```
levelrail-cli proxy [--domain D | --app NAME] [--proxy traefik|nginx|caddy] [--verify] [flags]
levelrail-cli proxy setup [--confirm] [--dynamic-dir DIR] [flags]
levelrail-cli proxy status [flags]
levelrail-cli proxy apply [flags]
levelrail-cli proxy verify [--domain D] [flags]
levelrail-cli proxy disable [flags]
```

Without a subcommand: shows which container publishes ports 80 and 443 and the configuration to paste into that proxy, for the dashboard (`--domain`) or an app's domain (`--app`, upstream is the ingress HTTP port). `setup` detects a Traefik (Coolify's included) and, with `--confirm`, turns on `tls_terminated_upstream`, saves the detected entrypoints, certificate resolver, directory and upstream host, writes one route file per domain and verifies each over HTTPS; without `--confirm` it is a dry run. It exits with an API error and the precise gap (`detection_incomplete`, `upstream_unreachable` with the drop-in fix, `not_writable`) when it cannot proceed. `status` prints detection, per-domain state (`missing`, `written`, `stale`, `error`), certificate issuer and expiry, and the checklist. `apply` rewrites the files now, `verify` probes again, `disable` removes the files it wrote. Writes need a root scoped token. Details: [Run behind an existing proxy](behind-an-existing-proxy.md#one-step-setup-traefik-including-coolify).

## Containers

```
levelrail-cli containers [list] [flags]
levelrail-cli containers stop <name> [flags]
levelrail-cli containers remove <name> [flags]
levelrail-cli containers claim <name> [--as app-name] [flags]
levelrail-cli containers orphans [flags]
levelrail-cli containers reap [--dry-run] [flags]
```

`list` shows every container on this node, managed or not. `stop`, `remove` and `claim` only act on a container the platform does not already manage (409 otherwise); for a managed app use `apps stop`, `apps start`, `apps restart` or `apps delete`. `claim` adopts an orphaned container into a new app. `orphans` lists leftover containers, volumes and certificates that no app or database accounts for, with where each stands against its grace period. `reap` runs one removal pass now (`--dry-run` reports without removing; exit 1 if anything could not be removed). Details and the `APP_ORPHAN_*` settings are in [Delete and clean up](deploying-apps.md#delete-and-clean-up).

## System maintenance

Fleet wide cleanup. These commands need an admin or root scoped token.

### System prune

```
levelrail-cli system-prune [flags]
```

Removes every stopped container, dangling image, and unused anonymous volume or build cache not part of the reconciler's current desired state, fleet wide. It never touches a named volume (an app's storage attachment, a database's data volume), even one that is actually orphaned: see [Orphaned volumes](#orphaned-volumes) for those.

### Control plane backups

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
levelrail-cli control-plane-backups help-dr
```

Snapshots of the control plane's own database, stored under `<data dir>/control-plane-backups/`. `create` takes one now (manual snapshots are never auto-deleted), `download` saves one to `--out FILE` (or raw bytes to stdout), `verify` re-checks the checksum, SQLite integrity and schema version without restoring (exit 1 if any check fails), `delete` removes one. Snapshots never contain the master key. Restore is an offline server command, `levelrail restore-db <file>`; see [Control plane backup and restore](/control-plane-backup).

The `--offbox`, `schedule`, `run-now`, `drill`, `escrow` and `keys` subcommands drive encrypted off-box backups. `keys generate` makes an age key pair on your machine (private key to a `0600` file, public key to stdout). `schedule set` changes only the flags you pass (`--enable`, `--disable`, `--destination`, `--recipient`, `--schedule`, `--drill-schedule`, `--retain-daily`, `--retain-weekly`, `--retain-monthly`, `--escrow-destination`). `run-now` and `drill run` wait for the run and exit 1 if it failed; `drill status` exits 1 when the last drill failed or none has run. `escrow` writes the master key and agent CA key encrypted to your recipients (never uploaded unless `--upload`, and never to the backup bucket). `help-dr` prints a static runbook. Restore an off-box backup on the server with `levelrail restore --from <s3://... | file> --identity FILE [--dry-run]`. See [Disaster recovery](/disaster-recovery).

### Orphaned volumes

```
levelrail-cli volumes-orphaned [flags]
levelrail-cli volumes-orphaned-cleanup --names name1,name2 [flags]
```

Named Docker volumes (an app's storage attachment, a database's data volume) survive `system-prune` even after the app or database that created them is deleted, since Docker never removes a named volume on its own. `volumes-orphaned` lists every one this instance created that no current app, database or storage attachment references any more. `volumes-orphaned-cleanup` removes exactly the volumes named with `--names` (comma separated), after the control plane re-confirms each one is still genuinely orphaned. There is no flag that deletes every orphaned volume unseen; review the list first.

## Users

```
levelrail-cli users list [flags]
levelrail-cli users create --email EMAIL --password PASSWORD (--role ROLE | --abilities LIST) [flags]
levelrail-cli users set-abilities <id> (--role ROLE | --abilities LIST) [flags]
levelrail-cli users roles [flags]
levelrail-cli users role set <id> <role> [flags]
levelrail-cli users grants get <id> [flags]
levelrail-cli users grants set <id> [--environment ID ...] [flags]
levelrail-cli users delete <id> [flags]
levelrail-cli invites create --email EMAIL (--role ROLE | --abilities LIST) [flags]
levelrail-cli invites list [flags]
levelrail-cli invites revoke <id> [flags]
```

`--role` is a curated preset (`admin`, `operator`, `viewer`, `guest`, or a custom role by name; `users roles` lists the curated presets and `roles list` lists every stored role); `--abilities` is a comma separated list from `read`, `read:sensitive`, `write`, `write:sensitive`, `deploy`, `root`. `users role set` changes a user's role; `users grants` manages which environments a guest can see. `invites create` invites a teammate by email; `invites list` and `invites revoke` manage pending invites. See [Access control](access-control.md) for details.

## Roles

```
levelrail-cli roles list [flags]
levelrail-cli roles create <name> --abilities LIST [--visibility all|granted] [--description TEXT] [flags]
levelrail-cli roles update <role> [--name N] [--abilities LIST] [--visibility V] [--description T] [flags]
levelrail-cli roles delete <role> [flags]
```

Custom roles are presets over an ability list. Updating a role applies the new abilities to every user holding it, immediately. A role cannot be deleted if any user holds it. See [Access control](access-control.md).

## IAM

```
levelrail-cli iam policies create --name NAME --document DOC [flags]
levelrail-cli iam policies list [flags]
levelrail-cli iam policies get <id> [flags]
levelrail-cli iam policies update <id> --name NAME --document DOC [flags]
levelrail-cli iam policies delete <id> [flags]
levelrail-cli iam policies attach <id> --principal-type user|token --principal-id ID [flags]
levelrail-cli iam policies detach <id> --principal-type user|token --principal-id ID [flags]
levelrail-cli iam policies attachments <id> [flags]
levelrail-cli iam templates list [flags]
levelrail-cli iam templates apply <id> [--param KEY=VALUE ...] [--name N] [--attach-user U | --attach-token T] [flags]
```

Resource scoped Allow and Deny policies, additive on top of `--abilities`. `DOC` is a policy document JSON string, passed inline or as `file://path/to/policy.json`:

```
levelrail-cli iam policies create --name read-web --document '{"Statement":[{"Effect":"Allow","Action":["read"],"Resource":["app:web"]}]}'
```

`update` replaces the policy's name, description and document. `attachments` lists a policy's attached principals. `templates` shows built-in policy templates and `apply` instantiates one, optionally attaching it to a principal. See [Access control](access-control.md).

## Environments

```
levelrail-cli environments list [flags]
levelrail-cli environments create --name NAME [--kind dev|test|uat|production|custom] [--protected] [--sort-order N] [flags]
levelrail-cli environments update <id> [--name N] [--kind K] [--protected[=false]] [--sort-order N] [flags]
levelrail-cli environments delete <id> [--move-to ID] [flags]
levelrail-cli apps move-env <app> <environment> [--confirm] [flags]
levelrail-cli databases move-env <db> <environment> [--confirm] [flags]
```

Global environments exist across the entire instance (Development, Test, UAT, Production). Kinds control which environment a resource has and what policies apply. Moving into or out of a protected environment requires confirmation and approval from another user. See [Environments](environments.md).

## Secrets

```
levelrail-cli secrets generate-master-key --out PATH
levelrail-cli secrets rotate-master-key --new-key-file PATH [flags]
levelrail-cli secrets binding-status [flags]
levelrail-cli secrets rebind [flags]
```

`generate-master-key` writes a new master key to a file (mode 0600, never overwrites), locally, without contacting the control plane. `rotate-master-key` re-wraps every stored data encryption key under a new master key in one atomic step, live, then binds any legacy values. `binding-status` counts stored secret values not yet bound to their slot, and `rebind` binds them (safe to rerun). Read [Master key rotation](master-key-rotation.md) before running these in production.

## Migrate

```
levelrail-cli migrate coolify --url URL --token TOKEN [flags]
levelrail-cli migrate dokploy --url URL --token TOKEN [flags]
levelrail-cli migrate caprover --url URL --token TOKEN [flags]
```

One way migration from a live instance. By default it writes `app.yaml` files plus a migration report into `--out-dir` (default `./migrated`); `--apply` creates the apps directly on a target instance instead, using `--target-token`, `--target-api-url` and `--target-profile`. `--include-secret-values` also fetches real env var values. For Coolify, `--token` is an API bearer token (with the `read:sensitive` ability if you want secret values); for Dokploy it is an API key; for CapRover it is the login password.

## Import

```
levelrail-cli import <repo-url|image> [--deploy] [--name NAME] [--ref REF] [--port N] [--env KEY=VALUE]... [flags]
levelrail-cli import -f compose.yaml|Dockerfile [flags]
levelrail-cli import --docker-run "docker run -p 80:80 nginx:1" [flags]
levelrail-cli import platform coolify|dokploy|caprover --url URL [flags]
```

`import` classifies its input (a GitHub, GitLab, Gitea or Bitbucket repo URL, a `docker run` command, an image reference, a docker-compose.yml or a Dockerfile) and prints a deployment plan. Nothing is created unless `--deploy` is given. `--file -` reads from stdin. `--env KEY=VALUE` is repeatable, and a required variable with no value blocks `--deploy`.

`import platform` reads apps, databases and settings from another platform (read only) and creates them here. Use `--dry-run` first. Databases and volumes are created empty; data is not migrated. Re-running skips what was already imported. The source token is read from `APP_IMPORT_SOURCE_TOKEN`, `--token-stdin` or `--token` (which lands in shell history). Other flags: `--only`, `--collision suffix|skip`, `--insecure-tls`, `--allow-private`, `--allow-loopback`. See [migrating from Coolify, Dokploy or CapRover](migrating-from-coolify-dokploy-and-caprover.md).

## Completion

```
levelrail-cli completion bash|zsh|fish
```
Prints a shell completion script for command and subcommand names and the global flags. It does not complete flag values or positional arguments like app names. For bash, `source <(levelrail-cli completion bash)`; run `levelrail-cli completion -h` for the zsh and fish install lines.

## Settings

Instance wide configuration, gated at the root ability server side on every write.

```
levelrail-cli settings oauth list [flags]
levelrail-cli settings oauth set <provider> [flags]
levelrail-cli settings email get [flags]
levelrail-cli settings email set [flags]
levelrail-cli settings ingress get [flags]
levelrail-cli settings ingress set [--primary-domain D] [--acme-enabled] [--public-https-port N] [--tls-terminated-upstream] [--apps-base-domain HOST] [--dns-cname-target HOST] [--dns-ttl SECONDS] [--dns-proxied] [flags]
levelrail-cli settings ingress https status [flags]
levelrail-cli settings ingress https enable --email EMAIL [--staging] [--wait 2m] [flags]
levelrail-cli settings dashboard-url get [flags]
levelrail-cli settings dashboard-url set --url URL [flags]
levelrail-cli settings updates get [flags]
levelrail-cli settings updates set --channel CHANNEL [--auto-update] [flags]
levelrail-cli settings deploy-freeze show [flags]
levelrail-cli settings deploy-freeze set --cron EXPR --duration D [flags]
levelrail-cli settings deploy-freeze clear [flags]
levelrail-cli settings ai-assistant get|set|clear [flags]
```

- `oauth set <provider>` enables, configures or disables one sign in provider; `<provider>` is `google`, `github` or `oidc`.
- `ingress get` shows the primary domain, ACME settings, the automatic hostname toggle and the detected public address. `ingress set` changes them (`--primary-domain`, `--acme-enabled`, `--acme-email`, `--acme-directory-url`, `--fallback-domains=false`, `--hsts-enabled`, `--apps-base-domain`, `--dns-cname-target`, `--dns-ttl`, `--dns-proxied`); flags you leave out keep their current value.
- `ingress https status` shows whether the dashboard's free sslip.io HTTPS is off, pending, issued or failed. `ingress https enable` points the dashboard at `<dashed-ip>.sslip.io` and issues a real Let's Encrypt certificate for it, with no DNS setup.
- `dashboard-url set` sets the public dashboard URL; once it is `https://`, sign in over plain HTTP is refused (`--url ""` clears it).
- `updates` configures the release channel and auto update checking. `deploy-freeze` manages fleet wide freeze windows; per app windows are under `apps freeze`.
- `ai-assistant` is experimental (`APP_EXPERIMENTAL=ai-chat`): `get` shows the current settings (the key itself is never returned, only whether one is stored), `set --model NAME --api-key KEY` configures the assistant, and `clear` clears the stored key and resets provider and model.

## Git integrations

```
levelrail-cli git-providers [flags]
levelrail-cli github-app status|disconnect|repos [flags]
levelrail-cli github-app branches <owner> <repo> [flags]
levelrail-cli github-app use-as-source <owner> <repo> --app-name NAME [flags]
levelrail-cli github-app register-url [--owner ORG] [--public] [--instance-url URL] [--name NAME] [flags]
levelrail-cli github-app installations list|add [flags]
levelrail-cli github-app installations remove <id> [flags]
levelrail-cli gitlab-app status|disconnect|projects [flags]
levelrail-cli gitlab-app branches <project-id> [flags]
levelrail-cli gitlab-app use-as-source <project-id> --app-name NAME [flags]
levelrail-cli bitbucket-app status|disconnect|repos [flags]
levelrail-cli bitbucket-app branches <workspace> <repo-slug> [flags]
levelrail-cli bitbucket-app use-as-source <workspace> <repo-slug> --app-name NAME [flags]
levelrail-cli gitea-app status|disconnect|repos [flags]
levelrail-cli gitea-app branches <owner> <repo> [flags]
levelrail-cli gitea-app use-as-source <owner> <repo> --app-name NAME [flags]
```

- `git-providers` shows connection status and capabilities (list branches, register a webhook, authenticated clone) for GitHub, GitLab, Bitbucket and Gitea in one call.
- Connecting a provider is dashboard only, because it is a browser redirect through the provider's own OAuth or manifest flow. Once connected, these commands browse its repos and branches, connect one as an app's git source with `use-as-source`, or check and forget the connection.
- `disconnect` forgets the stored connection locally; it does not uninstall the App or revoke the token on the provider's side.
- `github-app register-url` prints the dashboard link that starts GitHub App registration, for your personal account or, with `--owner`, an organization. `--public` lets other accounts and organizations install the App. It makes no request; open the link in a signed-in browser.
- `github-app installations add` prints the URL to install the App on another account or org; it does not open a browser. `installations remove` is refused (409) while a git source still points at a repo under it.

## Templates

```
levelrail-cli templates list [--custom] [flags]
levelrail-cli templates get <id> [flags]
levelrail-cli templates deploy <id> [--name NAME] [flags]
levelrail-cli templates delete <id> [flags]
```
`list` browses the curated service catalog, or `--custom` for your own saved templates. `get` shows one entry including its full compose.yaml. `deploy` deploys a template's compose.yaml as an app, and the app name defaults to the template id; pass `--name` to use another. A custom template works with every verb the same as a built-in one, except `delete`: only a custom template can be deleted (see `apps save-as-template`). See [Templates](templates.md).

## Static sites

```
levelrail-cli static-sites list [flags]
```
Lists every `build.type: static` app served directly through the embedded Caddy ingress, with no container involved.

## Related

<CardGroup :cols="2">
<Card title="Getting started" href="/getting-started">

Install the control plane and deploy a first app.

</Card>
<Card title="Deploying and managing apps" href="/deploying-apps">

The app lifecycle, command by command.

</Card>
<Card title="App spec reference" href="/app-spec-reference">

Every field of `app.yaml`.

</Card>
<Card title="Feature catalog" href="/feature-catalog">

The complete feature overview.

</Card>
</CardGroup>
