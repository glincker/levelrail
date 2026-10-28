---
title: "Levelrail CLI Reference"
description: Complete reference of all Levelrail CLI commands, organized by group with examples and typical use cases.
"og:image": "https://docs.levelrail.com/public/og/cli-reference.png"
"twitter:image": "https://docs.levelrail.com/public/og/cli-reference.png"
"og:url": "https://docs.levelrail.com/cli-reference"
---

`levelrail-cli` is a scriptable client for the control plane's HTTP API. Everything it does goes through `/api/v1`, with the same tokens and permissions as the dashboard, so you need no SSH key and no open port on your servers. This page lists every command group with its subcommands and the flags you will use most. Run `levelrail-cli` with no arguments for the command list, or `levelrail-cli <command> <subcommand> -h` for any command's own flags.

Commands are shown as `levelrail-cli ...`. If you rename the binary, the command name follows it (the CLI reads its name from `os.Args[0]`). The server is a separate binary, `levelrail`, which has a few maintenance commands of its own, such as `setup-token`, `restore-snapshot`, `restore-db`, `recover-admin`, `upgrade-note` (record who swapped the binary by hand), and `healthcheck`. Those are in [Installing](/installing) and [Disaster recovery](/disaster-recovery).

## Install and sign in

`curl -fsSL https://levelrail.com/install-cli.sh | sh`

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

Login pins the server. `auth login` prints the server's name and fingerprint (compare it with the one on the approval page) and saves its identity in the profile. Every later request first checks that the URL still answers as that server and refuses to send the token otherwise, naming the fix: `auth login --reauth` to log in to the new server, or `auth trust-server` to pin the one now answering. A profile saved before pinning existed asks once to confirm the fingerprint (`--yes` or `APP_TRUST_SERVER=yes` for scripts). `APP_API_TOKEN` keeps working and only prints a warning when the profile pins a different server. `auth status` and `auth whoami` show the pinned server.

Manage more than one control plane with named profiles: `levelrail-cli auth login --profile staging`, then `--profile staging` on any command. `levelrail-cli profile list` shows them.

## Updating the CLI {#updating}

```bash
levelrail-cli update --check          # current version, latest release, update available or not
levelrail-cli update --dry-run        # show what would change, download nothing
levelrail-cli update --yes            # replace this binary with the latest release
levelrail-cli update --version v0.8.2 --yes   # pin a tag, also works as a downgrade
levelrail-cli update --check --exit-code      # exit 10 when an update is available, for scripts
levelrail-cli update --channel beta --yes     # include prereleases
```

`self-update` is an alias of `update`, and `version --check` is a shortcut for `update --check`. `update --check` exits 0 when the check itself worked, even when you are out of date, unless you add `--exit-code`: then it exits 10 when an update is available. It honors `--json` and `--output`. `--channel stable|beta` (or `LEVELRAIL_CHANNEL`) works like `install-cli.sh`: `stable` only considers stable releases, `beta` also considers prereleases, and with neither the newest stable release wins, falling back to a prerelease only when no stable one ships your binary. The previous binary is kept next to the new one as `levelrail-cli.bak`; to roll back run `mv levelrail-cli.bak levelrail-cli` in that directory. When a package manager owns the binary (Homebrew, Nix, `/usr/bin`) the command refuses and names the right upgrade command; `--force` overrides that. On a non-writable directory it prints the `sudo` and `install-cli.sh` commands to use. `update` downloads the asset for your OS and architecture, verifies its checksum and the release's Sigstore signature, runs the new binary once to confirm its version, then atomically replaces the running file. If the signature or checksum cannot be verified, or the directory is not writable (a package manager path, or a root-owned `/usr/local/bin`: update it that way or re-run with `sudo`), nothing is changed. In CI mode or without a terminal it needs `--yes`. Windows is not supported in place: re-run `install-cli.ps1`.

Interactive runs print one line to stderr when a newer release exists, at most once per interval. It never runs in CI mode, with `--json` output, or for `help`, `version` and `update`, and it is cut off after a short wait so it cannot slow a command.

| Environment variable | Meaning | Default |
| --- | --- | --- |
| `LEVELRAIL_NO_UPDATE_CHECK` | `1` turns the hint off | unset |
| `LEVELRAIL_UPDATE_CHECK_DAYS` | days between hints, `0` disables | `7` |
| `LEVELRAIL_UPDATE_HINT_WAIT_MS` | longest the hint may wait after a command ends | `750` |
| `LEVELRAIL_CHANNEL` | `stable` or `beta`, same as `--channel` | unset |
| `LEVELRAIL_RELEASE_API_URL` | releases API base | `https://api.github.com` |
| `LEVELRAIL_RELEASE_BASE_URL` | asset download base, https only | the release repo's GitHub releases |
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
| 10 | `update --check --exit-code` found a newer CLI release (the check itself worked) |

Codes 3 and 4 are broad on purpose so existing scripts keep working; the JSON error object carries the finer distinction.

## CI mode

The CLI reads `LEVELRAIL_TOKEN`, `LEVELRAIL_API_URL` and `LEVELRAIL_PROFILE` as env-only auth. Precedence: flag, then these, then `APP_API_TOKEN` / `APP_API_URL` / `APP_PROFILE`, then the saved profile.

With `CI=true`, `LEVELRAIL_CI=1`, or stdin or stdout not a terminal, the CLI never prompts (a command that needs input exits non-zero and names the flag or env var to set), prints no color and shows no animation. `NO_COLOR` is honored everywhere. `auth whoami` prints a `source:` line (flag, env name or profile), never the token.

## Scripting: `--debug`

`--debug` works anywhere on the command line (before or after the subcommand) and traces every outgoing request this invocation makes to stderr: method, URL, and the response status and timing, one line per call. It never prints request or response headers or bodies, so `Authorization` and any token value never appear, even in debug mode; a token that somehow ended up in a URL's query string is also redacted. stdout is untouched, so `--debug` composes with `--json`/`--query` for scripting.

```
levelrail-cli apps list --debug --api-url http://10.0.0.5:8080
```
```
DEBUG: GET http://10.0.0.5:8080/api/v1/apps -> 200 OK (42ms)
```


## Experimental command groups

Some groups are off by default and stay hidden from `--help` and shell completion until the shell you run the CLI from sets `APP_EXPERIMENTAL`. This page marks them. The keys are `ai-chat` (`ai`, `settings ai-assistant`), `ai-models` (`models`), `load-balancer` (`lb`), `iac` (`apply`, `diff`, `export`) and `cloudflare-tunnel` (`cloudflare-tunnel`). See [Experimental features](/experimental-features).

## Web CLI console: host diagnostics

The dashboard's CLI console (experimental key `cli-console`) runs the commands on this page against the control plane. It also answers a small `host` namespace itself, inside the control plane process, without starting the CLI or any other program. It is read only and is not a shell.

| Command | Shows |
| --- | --- |
| `host df` | Disk usage of the data directory and `/` |
| `host free` | Memory total, used, available, and swap |
| `host uptime` | Time since the server booted |
| `host load` | 1, 5 and 15 minute load averages and the CPU count |
| `host os` | OS name (`PRETTY_NAME` from os-release), kernel release and architecture |
| `host service` | The control plane's version, start time, uptime, goroutines, memory (RSS) and whether a restart supervisor (systemd, Docker) was detected |
| `host help` | This list |

None of them take arguments: paths, process names, container names and flags are refused. Each line goes through the same step-up, one-command-at-a-time, idle timeout, output cap and audit log (`cli_console.run`) as any other console command. Linux only; other platforms reply "not supported on this platform". Set `APP_CLI_CONSOLE_HOST=off` on the server to turn the namespace off while keeping the rest of the console.

## Naming conventions

A command's own subject is a positional argument or `--name`. `--app` is a scope flag: it narrows a list or ties a new object to an app (`alerts history --app web`, `proxy --app web`), and is never the name of the thing being created. Every list command prints a bare JSON array under `--json`; paging info (`apps list --max-items`) goes to stderr, so the stdout shape never changes with a flag.

## Simple verbs

`levelrail-cli help` lists the 13 everyday verbs (`up`, `deploy`, `rollback`, `logs`, `env`, `domains`, `db`, `backup`, `restore`, `apps`, `status`, `open`, `login`). `levelrail-cli help advanced` lists everything else. There is one CLI: the simple verbs are shortcuts over the same commands, and every flag of the longer forms still works.

```
levelrail-cli up [dir] [--name N] [--port P] [--domain D] [--with postgres,redis] [--replicas N] [--strategy S] [--memory 512Mi] [--cpu 0.5] [--repo URL] [--ref BRANCH] [--secret KEY=VALUE] [--allow-unpushed] [--no-wait] [--timeout D] [--dry-run] [--yes] [flags]
```
Deploy a directory with one command. See the [one command quickstart](/quickstart-one-command).

```
levelrail-cli deploy <name> --image IMAGE [flags]
```
Alias for `apps deploy`; deploy an image to an existing app.

```
levelrail-cli rollback <name> [--image IMAGE] [flags]
```
Alias for `apps rollback`; without `--image` it goes back to the previous successful deploy.

```
levelrail-cli logs <app> [flags]
```
Same as `apps logs <app>`. The log archive and search commands keep their names (`logs archive`, `logs search`, `logs dump`).

```
levelrail-cli env set <app> KEY=VALUE... [--apply] [flags]
```
Set plain environment variables. Keys stored as secrets are refused; use `apps secrets set` for those.

```
levelrail-cli open <app> [flags]
levelrail-cli login [--device] [flags]
levelrail-cli backup <database> [--target ID] [flags]
levelrail-cli restore <database> (--backup ID | --latest) --confirm NAME [flags]
```
`open` prints the app's public URLs. `login` is `auth login`. `backup` is `backups trigger` and uses the only configured backup target when there is exactly one. `restore` is `backups restore` and `--latest` picks the newest succeeded backup.

## Local instance

```
levelrail-cli dev [--port N] [--data-dir DIR] [--no-open] [--reset] [--bin PATH]
levelrail-cli dev status [--json]
levelrail-cli dev stop
```
Run a private control plane on loopback with a generated admin and `<app>.localhost` addresses. See [Run it on your laptop](/local-development).

## Apps

`apps list` prints every app the caller can read as a bare JSON array. On a fleet large enough that matters, add `--max-items N` to cap the page size; stdout stays a bare array and the paging info goes to stderr as `next_token=N total_count=M`; an empty `next_token` means there is no further page. Pass that `next_token` back in as `--starting-token` to fetch the next page (requires `--max-items`, since the server only pages a result when a limit is set):

```
levelrail-cli apps list --max-items 50 --json
levelrail-cli apps list --max-items 50 --starting-token 50 --json
```

Run `levelrail-cli apps <subcommand> -h` for the full flag list of any command below. Deploy lifecycle is covered in [Deploying and managing apps](/deploying-apps).

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

- `apps create` registers an existing image or triggers a real build from a git repository; `--file` reads an [app spec](/app-spec-reference), `--interactive` runs a step-by-step wizard. Required flags missing outside `--interactive` fail with an error instead of prompting. Flags-only creates can set health with `--readiness-path|exec|preset|interval|timeout|expected-status`, the same `--liveness-*` set plus `--liveness-failures`, and `--ready-timeout`; they follow the same rules as `apps health set`.
- `apps status` shows an app's current reconcile conditions; `apps overview` shows CPU, memory, traffic and errors for every app (or the names given) in one request; `apps network` shows the live traffic path (container port, host port, running).
- `apps create --file` sends a service's `published_ports` (container_port, host_port, protocol tcp or udp). `apps get`, `apps list` and `apps deploy-compose` show them as `53/udp`, and `--json` carries `published_ports`. Refusals print the server message plus a hint.
- `apps validate` parses and validates an app.yaml or a Docker Compose file locally, with no API call and no deploy. It prints the detected format, service count, and every non-blocking `notices` entry a real deploy would also surface.
- `apps delete` removes an app and stops its containers; if the node is unreachable the delete is reported as pending and retried.

### Deploy, roll back and scale

```
levelrail-cli apps deploy <name> --image IMAGE [flags]
levelrail-cli apps deploy-compose <name> --file compose.yaml [flags]
levelrail-cli apps deploy-spec <name> --file app.yaml --repo-url <url> --ref <ref> [flags]
levelrail-cli apps builds trigger <name> --repo URL --ref REF [flags]
levelrail-cli apps rollback <name> [--image IMAGE] [flags]
levelrail-cli apps promote <name> --to ENVIRONMENT_ID [--target NAME] [--preview] [flags]
levelrail-cli apps wait <name> [--attempt-id ID] [--timeout 5m] [--poll-interval 3s] [flags]
levelrail-cli apps restart <name> [flags]
levelrail-cli apps apply <name> [flags]
levelrail-cli apps stop <name> [flags]
levelrail-cli apps keep <name> [flags]
levelrail-cli apps start <name> [flags]
levelrail-cli apps scale <name> [--replicas N] [--strategy rolling|recreate|blue-green] [--memory 512Mi] [--cpu 0.5] [flags]
levelrail-cli apps group <name> [flags]
levelrail-cli apps hook-runs <name> [flags]
levelrail-cli apps images <name> [flags]
levelrail-cli apps set <name> --pr-comment-commands on|off [--pr-comment-commands-forks on|off] [flags]
levelrail-cli apps set-node <name> <node-id> [--with-volumes] [flags]
levelrail-cli apps clear-node <name> [--with-volumes] [flags]
levelrail-cli apps moves list <name> [flags]
levelrail-cli apps moves get <name> <id> [flags]
```

- `apps keep` turns a one-click trial deployment into a permanent app: it stops being flagged as throwaway, with every service of the same app. Repeating it is harmless; an app that never was a trial answers 409.
- `apps wait` polls until a deploy attempt converges and exits 0 on success, 5 if it converged as a failure, 6 on timeout. On success it says what happened: `rolled out`, `already up to date` (the deploy changed nothing) or `restarted`; `--json` carries the same as `outcome`.
- `apps deploy-spec` fans an app.yaml's `services:` map out into independent builds under one app; `apps group` shows a service's siblings under the same multi-service app.
- `apps builds trigger` builds an image from a git source and deploys it to an existing app.
- `apps promote` promotes the app's image onto a sibling app in another environment.
- `apps apply` restarts an app so saved env, secret and config changes reach the running container; it does nothing when nothing is pending.
- `apps scale` changes the replica count and/or the deploy strategy.
- `apps hook-runs` shows the most recent outcome of an app's pre and post deploy hooks; `apps images` lists the locally present image tags under the app's current image repo.
- `apps set` changes app settings. `--pr-comment-commands on|off` lets write-access users run `/levelrail` commands from pull request comments (see [Pull request comment commands](/pr-comment-commands)).
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
levelrail-cli apps deploys rollback-to <name> <deploy-id> [--watch] [flags]
levelrail-cli apps deploys gate <name> [deploy-id] [--watch] [flags]
levelrail-cli deploy status <name> [deploy-id] [--watch] [flags]
levelrail-cli apps deploy-safety show|set|targets <name> [flags]
levelrail-cli apps timeline <name> [--limit N] [flags]
levelrail-cli apps diagnose <name> [--deploy ID] [--apply-fix N] [flags]
levelrail-cli apps preflight <name> [--require-env A,B] [flags]
levelrail-cli apps freeze show <app-name> [flags]
levelrail-cli apps freeze set <app-name> --cron EXPR --duration D [--timezone TZ] [--reason TEXT] [flags]
levelrail-cli apps freeze clear <app-name> [flags]
levelrail-cli apps schedule set <app> --cron EXPR --branch NAME [flags]
levelrail-cli apps schedule set <app> --run-at 2h --branch NAME [flags]
levelrail-cli apps schedule get <app> [flags]
levelrail-cli apps schedule history <app> [flags]
levelrail-cli apps schedule list <app> [--status S] [flags]
levelrail-cli apps schedule cancel <app> <id> [flags]
levelrail-cli apps deploy <app> --at WHEN (--image IMAGE | --latest --branch NAME) [flags]
levelrail-cli apps cancel-superseded enable|disable|status <app-name> [flags]
levelrail-cli apps auto-rollback enable|disable|status <app-name> [flags]
levelrail-cli apps autoscale show|set|clear|history <app-name> [flags]
levelrail-cli apps abuse get|set|dry-run <app-name> [flags]
levelrail-cli functions init|deploy|list|runtimes|invoke|delete [flags]
levelrail-cli jobs list|get|create|update|run|history|logs|enable|disable|delete <app> [<job>] [flags]
levelrail-cli apps sleep enable|disable|status|wake|now|history <app-name> [flags]
levelrail-cli apps sleep list [flags]
levelrail-cli apps canary start|status|weight|promote|abort <app-name> [flags]   (start takes --stages, --interval, --max-error-rate, --max-p95-ratio for a staged canary)
levelrail-cli apps build-hooks list|create|rotate|revoke <app-name> [flags]
levelrail-cli apps auto-update enable|disable|status|check <app-name> [flags]
levelrail-cli apps auto-rollback-slo-burn set <app-name> off|auto|dry_run|pause_for_human [flags]
levelrail-cli apps auto-rollback-slo-burn status <app-name> [flags]
```

- `apps deploys list` is real, row per attempt deploy history, newest first. `apps deploys show` prints one attempt (the newest by default) with its structured failure: code, cause, failing step, redacted log excerpt, suggested fix, docs link and retryable. See [Deploy failures](/deploy-failures).
- `apps deploys wait` blocks until one deploy is healthy, failed, canceled, superseded or blocked (a freeze window or approval) and prints the result with its failure. Exit 0 healthy, 7 failed, canceled, superseded or blocked, 6 timeout. Without a deploy id it resolves the newest deploy once and follows it by id.
- `apps deploys compare` diffs two deploy attempts, or one against the current live state. `apps deploys logs` prints one attempt's full build and log output to stdout. `apps deploys steps` streams the pipeline steps (detecting, building, pushing, deploying) until the attempt ends and exits non-zero if a step failed; an already finished attempt replays only a short summary.
- `apps deploys failed` lists every app's latest failed deploy in the window (default set by the server), with the image of its newest good deploy as a rollback target.
- `apps deploys cancel` cancels a queued or in-progress deploy before it cuts traffic. `apps deploys rollback-to` redeploys a past succeeded deploy's exact image, pinned by digest or rollback pin, with that deploy's config; `--watch` follows it through the readiness gate.
- `apps deploys gate` (alias `deploy status`) prints a release's gate timeline: readiness passes, stable window, cutover, drain, rollback pin and post-deploy watch. `--watch` follows it until the release clears its watch, fails, is rolled back or is replaced. Exit 0 healthy or in progress, 7 failed or rolled back. `apps deploy-safety` shows and overrides the per-app thresholds (`set web --stable-window 20s --auto-rollback false`, `default` clears one) and lists pinned rollback targets. See [Deploy safety](/deploy-safety#readiness-gate).
- `apps timeline` shows what happened to an app, newest first: deploys, rollbacks, restarts, env, secret and config changes (key names only, never values), scaling, stop and start.
- `apps diagnose` explains a failed deploy or crashloop and can apply a suggested fix with `--apply-fix N`. `apps preflight` runs pre-deploy checks (DNS, ports, disk, image, env).
- `apps deploy`, `apps rollback`, `apps restart`, `apps start` and `apps stop` exit non-zero with code `node_offline` when the app's node is offline, naming the node and the fix (bring it back, or `apps move` / `nodes drain`). `--queue-while-offline` records the change as a `pending` attempt that the node applies when it returns.
- `apps freeze` manages deploy freeze windows: a window starts at every match of a 5 field cron expression, evaluated in `--timezone` (default UTC), and lasts `--duration`. While one is active, webhook, pipeline and released deploys are held and run when the window ends; a manual deploy needs `apps deploy --override-freeze --override-reason TEXT`. `set` replaces the app's windows with the one given. Fleet-wide windows are under `settings deploy-freeze`.
- `apps schedule` configures a recurring redeploy of a branch's latest commit on a cron schedule. `apps deploy --at` schedules a one-shot image or git ref deploy, and `apps schedule list` and `apps schedule cancel` manage those (see [Scheduled deploys](/scheduled-deploys#one-shot-scheduled-deploys)).
- `apps cancel-superseded` lets a newer queued deploy replace older queued ones of the same branch. Only queued deploys are replaced; a deploy that already started building is never canceled automatically.
- `jobs` manages an app's scheduled jobs: fresh-container or exec cron jobs with run history, logs, retries and catch-up (see [Scheduled jobs](/scheduled-jobs)). `apps scheduled-tasks` is the older exec-only form of the same resource.
- `functions` runs a handler repo (built with Railpack) or an image as a function: it sleeps when idle, and a request to a sleeping one waits through the cold start. `functions init node|python|go` writes a ten line starter, `functions list` shows state, calls and p95 duration (see [Functions](/functions)).
- `apps sleep` stops an app after it has had no requests for a set time and wakes it on the next request (see [Sleep when idle](/sleep-when-idle)).
- `apps canary` runs a new image beside the app and sends it a share of traffic until you promote or abort it (see [Canary deploys](/canary-deploys)).
- `apps auto-update` redeploys an app automatically when its image tag moves to a new digest (see [Image auto-update](/image-auto-update)).
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

- `apps domains add` creates the DNS record when a provider is connected (`--dns off` skips it, `--dns preview` shows it); `--wait` prints the go-live steps and exits non-zero when the domain is not live within `--timeout`. See [Add a domain and go live](/domains-and-ingress#add-a-domain-and-go-live).
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
- `apps tunnel <app> [--service s] [--port p] [--local-port n]` listens on 127.0.0.1 and relays to a private container port, see [app tunnel](/app-tunnel).
- `apps logs` searches an app's stored log entries, or streams live with `--follow`. For a capped excerpt see `logs query` under [Logs](#logs).
- `apps requests` shows request rate, errors and latency from the ingress; `apps resource-usage` ranks every app by latest CPU, memory and network usage.
- `apps resource-recommendation` suggests memory and CPU limits from historical usage; `apps cost` estimates what the app's CPU and memory would cost under reference providers (not a real bill).
- `costs explore|budgets list|create|set|delete` is the cost explorer with forecast and top movers, and named budgets with alert thresholds and an optional pause or throttle at 100 percent, see [cost explorer](/cost-explorer).
- `usage report|settings|budgets` shows cost showback and manages prices and budgets, see [usage and cost](/usage-and-cost). `usage report --csv` prints the export. `budget set|delete project|team ID` is an alias for `usage budgets`. `budget show|set|clear APP` manages a per-app egress and cost cap, see [spend caps](/spend-caps).
- `advisor findings|summary|apply|dismiss|snooze|restore|enable|disable` drives the beta [resource advisor](/resource-advisor). `advisor apply ID` only previews; add `--yes` to apply.
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
levelrail-cli apps build-stats <name> [--limit N]
levelrail-cli apps build-speed <name> [--fix]
levelrail-cli apps build-limits show|set|reset
levelrail-cli apps build-cache remove <name>|--global
```

- `apps health set` sets one probe and keeps the other. A probe is an HTTP(S) check or a command run inside the container (exit 0 is healthy). `--preset` fills path, interval, timeout and failures from a shortcut (`healthz`, `health`, `api-health`, `ping`, `status`); explicit flags win. `apps health discover` probes well-known paths and reports what each one did.
- `apps volumes` manages named Docker volumes outside a redeploy. The name is lowercase letters, numbers and hyphens starting with a letter; the path is where it mounts in the container.
- `apps storage set` attaches a connected bucket as object storage and injects its credentials as `S3_*` env vars at container create time. See [Object storage](/object-storage).
- `apps build-cache` configures the BuildKit remote cache on a storage destination. `--global` applies to every app without its own setting. `clear` is bounded per call; run it again while it reports more to delete. A cache problem never fails a build.

### Environment, secrets and Vault

```
levelrail-cli apps env import <name> --file .env [--dry-run] [--keep-existing] [--apply] [flags]
levelrail-cli apps env export <name> [--out FILE] [--reveal] [--format dotenv|json] [flags]
levelrail-cli apps env get <name> <KEY> [--reveal] [flags]
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
levelrail-cli apps branch-deploys list <name> [flags]
levelrail-cli apps branch-deploys set <name> --mode off|all|patterns [--branch PATTERN]... [flags]
levelrail-cli apps branch-deploys delete <name> <slug> [flags]
levelrail-cli apps contexts list|clear|show <name> [flags]
levelrail-cli apps contexts set <name> --context CTX [--env K=V]... [--build-command CMD] [--publish DIR] [--build-arg K=V]... [flags]
```

- `apps env import` merges a .env file into an app's plain env vars and prints which keys are new, changed or unchanged. Keys that are already secrets are skipped. It reports how many changes are pending, or restarts the app right away with `--apply`.
- `apps env diff` compares two apps' env vars, for example staging against production. Secret values are never read: secrets are compared by key only, and the dashboard has the same comparison on an app's Environment page.
- `apps env export` writes env vars as .env text; sensitive and secret keys are written empty with a comment unless `--reveal` is given. `apps env get` prints one variable, masked when sensitive. `--reveal` asks for your password (and authenticator code) at the prompt, never from flags or the environment, and needs a token with `read:sensitive` and `secrets:reveal` plus `APP_SECRET_REVEAL_TOKENS=on` on the server. See [Environment variables and secrets](/environment-and-secrets).
- `apps secrets set` sets or rotates one secret's encrypted value and declares the key as secret backed so it is injected; `--apply` restarts the app now. With `--env-file` it bulk imports every key in a .env format file as its own secret. Values are never returned: `list` shows key names and locked state only. `lock` toggles a secret's overwrite guard.
- `apps vault-env` declares an env var resolved live from the configured external Vault (see [Vault](#vault)); only the reference is stored.
- `apps preview-env` and `apps branch-env` override an env var for previews only. A branch override applies when the preview's own branch matches PATTERN (an exact name or a glob such as `release/*`); `--secret` stores it envelope encrypted. Neither affects the app's own running deploy.
- `apps branch-deploys` chooses which branches get a stable per-branch deploy (see [Preview environments](/preview-environments#branch-deploys)), lists the live ones and removes one now.
- `apps contexts` sets env and build overrides per deploy context and shows the effective config (see [Deploy contexts](/deploy-contexts)).

### Source, previews and webhooks

```
levelrail-cli apps git-source validate <name> [flags]
levelrail-cli apps git-source detect <name> [--apply|--package DIR] [flags]
levelrail-cli apps git-source affected <name> --files a,b [flags]
levelrail-cli apps git-source get <name> [flags]
levelrail-cli apps git-source set <name> --repo-url URL [flags]
levelrail-cli apps git-source settings <name> [flags]
levelrail-cli apps git-source rotate-secret <name> [flags]
levelrail-cli apps git-source register-webhook <name> [flags]
levelrail-cli apps git-source delete <name> [flags]
levelrail-cli apps webhook-deliveries list <app-name> [flags]
levelrail-cli apps webhook-deliveries replay <app-name> <delivery-id> [flags]
levelrail-cli apps previews list [app-name] [flags]
levelrail-cli apps previews limits [app-name] [flags]
levelrail-cli apps previews enable <app-name> [flags]
levelrail-cli apps previews disable <app-name> [flags]
levelrail-cli apps previews approve <app-name> <pr-number> --yes
levelrail-cli apps previews teardown <app-name> <pr-number> [flags]
levelrail-cli apps previews extend <app-name> <pr-number> [--hours 24] [flags]
levelrail-cli apps previews settings <app-name> [flags]
levelrail-cli previews list|delete|extend|settings|approve ...
levelrail-cli apps previews pr-status enable <app-name> [flags]
levelrail-cli apps previews pr-status disable <app-name> [flags]
levelrail-cli apps previews sweep [flags]
```

- `apps git-source` connects a repo for auto deploy on push; `settings` sets push path filters and forge status reporting; `rotate-secret` mints a fresh webhook secret, shown once.
- `apps webhook-deliveries` lists recent inbound webhook requests, verified or not. `replay` re-runs a stored delivery's payload and can trigger a real build and deploy.
- `apps previews` manages preview environments per pull request, see [Preview environments per pull request](/previews). `previews` is the same command at the top level. `extend` keeps one preview past its TTL; `settings` shows or changes the lifecycle, resource, database and exposure policy. `limits` shows or changes preview caps, fork policy and TTL; `approve` deploys a held fork pull request once; `teardown` removes one PR's preview now; `sweep` tears down every stale preview across all apps now; `pr-status` toggles the PR comment and commit status per preview deploy.

### Supply chain

```
levelrail-cli apps sbom <app> [deploy-id] [--download] [--file PATH] [flags]
levelrail-cli apps scan enable <app> [flags]
levelrail-cli apps scan disable <app> [flags]
levelrail-cli apps scan status <app> [deploy-id] [flags]
levelrail-cli apps scan run <app> [deploy-id] [flags]
levelrail-cli apps scan gate <app> off|warn|block_on_critical|block_on_high [flags]
levelrail-cli apps scan override <app> --reason TEXT [flags]
```

`apps sbom` shows a deploy's software bill of materials (newest deploy with one by default), or prints or saves the raw SPDX or CycloneDX document. Scanning is off by default and needs `APP_BUILD_ATTEST=true` on the server so builds produce an SBOM; a server can refuse every scan with `APP_SCAN_ENABLED=false`. `gate block_on_critical` keeps the previous release serving; `override` lets the next blocked release through once, with a recorded reason. See [Supply chain](/supply-chain).

### Alerts, connections and templates

```
levelrail-cli apps alerts list <app> [flags]
levelrail-cli apps alerts create <app> --kind KIND [flags]
levelrail-cli apps alerts update <app> <id> --kind KIND [flags]
levelrail-cli apps alerts delete <app> <id> [flags]
levelrail-cli apps alerts slo <app> [--slo 99.9] [--slo-latency-ms N]
levelrail-cli apps alerts template <app> [--profile-type web|worker|database] [--channel ID]
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

- `apps connect` is the canonical way to attach a database; `apps database set` keeps working for the single-database form. `apps connect` connects an app to a managed database and injects its resolved connection value as an env var; unlike `apps database`, an app can have any number of these. `connections list` shows each connection, including whether it resolves to a mesh DNS name (cross node capable) or a container name; `connections suggest` lists databases the app could connect to. `apps disconnect` removes one connection by its env var name.
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
- `apps projects topology` prints a diagram ready graph of a project's apps, databases and volumes. See [Projects and organizations](/projects-and-organizations).

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
levelrail-cli pipelines run <app> <name> [--ref R] [--sha S] [--param k=v]... [--follow] [flags]
levelrail-cli pipelines runs <app> [<run-id>] [--pipeline N] [--limit N] [flags]
levelrail-cli pipelines runs --all [--status S] [--app A] [--trigger T] [--pipeline N] [--limit N] [flags]
levelrail-cli pipelines logs <app> <run-id> [--job KEY] [--follow] [flags]
levelrail-cli pipelines cancel <app> <run-id> [flags]
levelrail-cli pipelines approve <app> <run-id> [--reject] [--comment TEXT] [--approval ID] [flags]
levelrail-cli pipelines sync <app> [--repo-truth=true|false] [flags]
levelrail-cli pipelines artifacts <app> <run-id> [--download JOB] [--out FILE] [flags]
levelrail-cli pipelines triggers <app> [flags]
levelrail-cli pipelines triggers <app> <pipeline> [--enable [--hmac]|--disable|--rotate|--delete|--deliveries|--replay ID] [flags]
levelrail-cli pipelines templates [<id>] [--out FILE] [--create APP [--name N]] [flags]
levelrail-cli pipelines fixes <app> <run-id> [--apply KIND --job J [--rerun]] [flags]
levelrail-cli pipelines insights <app> <pipeline> [flags]
levelrail-cli pipelines oidc [flags]
levelrail-cli pipelines oidc rotate-key [--retire-after D] [flags]
```

- `validate` checks a pipeline file locally with no API call and exits 2 when it has problems.
- `save` creates or updates pipelines from one file, or from every file in a repository's pipeline directory.
- `runs` lists runs, or shows one run's jobs, steps and approval gates; `--all` lists runs across every app.
- `run --param` sets a declared run parameter (`--input` is the older spelling).
- `artifacts` lists the archives a run stored, or downloads one job's archive.
- `triggers <app> <pipeline>` manages the pipeline's inbound webhook: the token and signing secret are shown once when created or rotated.
- `approve` decides approval gates, or releases a run held for approval.
- `sync` syncs pipeline files from the repository now, or sets the repository as source of truth.
- `triggers` shows why recent git events did or did not start runs.
- `oidc` shows whether pipeline jobs can mint OIDC tokens and the JWKS URL to wire to a cloud provider; `oidc rotate-key` rotates the signing key, and the old key stays published until it retires.

See [Pipelines](/pipelines) and [Pipeline OIDC](/pipelines-oidc).

## Load balancers {#lb}

Experimental: requires `APP_EXPERIMENTAL=load-balancer`. See [Load balancing](/load-balancing).

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

Deploy previews are the screenshot or metadata card captured after a deploy, not pull request previews (those are `apps previews`). See [Deploy previews](/deploy-previews).

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
levelrail-cli databases import-hosted <name> (--from-env VAR | --from-stdin) [--writes-frozen] [--allow-large] [--create-missing-roles] [--dry-run] [--json] [flags]
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
- `import-hosted` copies a hosted Postgres (such as Heroku) into an existing managed Postgres database. The source URL is read from the environment variable named by `--from-env` or from the first line of stdin, never from an argument. `--dry-run` runs the preflight only and prints blockers, warnings and info with the fix for each. Without `--writes-frozen` a real run refuses. Exit code 1 means the preflight is blocked or the move failed. See [Migration reference](/migration-reference#move-a-hosted-postgres-heroku).
- `delete` stops and removes a database but keeps its data volume; `--force` is needed if apps use it.
- `slow-queries` lists a Postgres or MySQL database's slow query log.
- `schema` lists a SQL database's tables with row estimates and sizes; `query` runs one statement through the control plane, read-only by default. See [Database viewer](/database-viewer). `db` is a short alias for `databases`.
- `set-resources` applies memory and CPU limits to an already created database, replacing whatever was set before (a full replace, not a patch).
- `public-access set` exposes a database on a host port; `--bind-address` is `private` (the default), `public`, or a literal IP.
- `set-version` is a minor or patch image change on the same data. `major-upgrade` is a guarded Postgres major upgrade with a rollback snapshot; `major-upgrades` lists the attempts, `major-upgrade-rollback` restores the pre upgrade data, and `major-upgrade-discard` deletes a rollback snapshot to free disk.

See [Managing databases](/managing-databases).

## AI models {#models}

Experimental: requires `APP_EXPERIMENTAL=ai-models`. See [AI models](/ai-models).

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
levelrail-cli models catalog [flags]
levelrail-cli models benchmark <name> [flags]
levelrail-cli models metrics <name> [flags]
levelrail-cli models gpus [flags]
levelrail-cli models preflight <repo> [flags]
levelrail-cli models cache list [flags]
levelrail-cli models cache prune [--dry-run] [volume...] [flags]
```

- `deploy` runs a model as a container on a GPU node and prints its API key once. Flags: `--engine` (`ollama`, `vllm` or `llamacpp`), `--model`, `--node`, `--gpus` (a count or `all`), `--gpu-devices`, `--context`, `--quantization`, `--domain`, `--hf-token-from-env`, `--residency` (`always` or `on_demand`), `--idle-ttl`, `--swap-group`. `--compute cpu` (or `--catalog ID`) runs a small GGUF model on the CPU with hard limits: `--cpus`, `--memory-mib`, `--weights-file`, `--accept-license` (see [AI models](/ai-models#cpu-models)). `catalog` lists the curated CPU models with licence, memory need and an estimated speed on this server. `benchmark` measures a running model. `fit --compute cpu` rates a model against the node's CPUs and memory.
- `logs` searches a model engine's logs, or `--follow` streams download and load progress live. `delete` keeps the downloaded weights volume. `restart` recreates the engine container.
- `rotate-key` issues a new API key, printed once; `keys` manages named API keys with limits, expiry and rotation grace; `usage` shows gateway requests, tokens, errors and latency per key.
- `residency` sets a model always resident or `on_demand` (stopped when idle, started on first request); `wake` and `sleep` start or stop an on demand engine now; `swap-group` lets models share a GPU (`--clear` removes it).
- `fit` estimates VRAM fit on each GPU node; `gpus` lists GPU nodes with VRAM and usage; `preflight` checks a Hugging Face repo (access, size, quantizations, fit, disk); `metrics` shows engine metrics.
- `cache list` shows cached model weights per node; `cache prune` removes unused, unreferenced weights (use `--dry-run` first).

## Auth

```
levelrail-cli auth login [--device] [--profile NAME] [flags]
levelrail-cli auth login --reauth [--device] [flags]
levelrail-cli auth whoami [--access] [flags]
levelrail-cli auth status [flags]
levelrail-cli auth trust-server [--yes] [flags]
levelrail-cli auth session-link [flags]
levelrail-cli auth code [flags]
levelrail-cli auth approve <id> --match N [flags]
levelrail-cli auth deny <id> [flags]
levelrail-cli auth devices [revoke <id>] [flags]
levelrail-cli auth code-login [--admins true|false] [--others true|false] [flags]
levelrail-cli auth activity [--kind KIND] [--all] [--limit N] [--cursor C] [flags]
levelrail-cli auth preferences get|set [--timezone Z] [--language L] [--theme T] [--landing-page P] [--gravatar] [flags]
levelrail-cli auth export [--out FILE] [flags]
levelrail-cli auth unlink <provider> [flags]
levelrail-cli auth email-change request --new-email ADDRESS [flags]
levelrail-cli auth email-change confirm --token TOKEN [flags]
levelrail-cli auth delete-account --confirm EMAIL [--code CODE] [flags]
levelrail-cli auth 2fa status [flags]
levelrail-cli auth 2fa setup [flags]
levelrail-cli auth 2fa enable --code CODE [flags]
levelrail-cli auth 2fa disable --code CODE|--recovery-code CODE [flags]
levelrail-cli auth 2fa recovery-codes --code CODE [flags]
```

- `login` authenticates and persists a new API token; with `--profile NAME` it saves under a named profile instead of overwriting `default`.
- `login --device --json` prints one JSON line first (`verification_url`, `user_code`, `expires_in`, `expires_at`) so an agent can relay it, then the token resource once approved. See [Attention center](/attention-center).
- `session-link` mints a short lived (about 2 minutes), single use login link that signs in as whoever minted it, meant for browser automation. It needs a token with the root ability.
- `code` prints the sign-in codes waiting for your account with the requester's IP address, browser and time, plus password sign-ins from new browsers waiting for approval; `approve` and `deny` decide one of those (`approve` needs `--match` with the two digit number the waiting browser shows, typed in; a wrong number denies it, and the number is never printed by `code`), and `devices` lists or revokes browsers trusted for password sign-in. They need a token that belongs to your user. Listing and `devices` accept `write:sensitive`; showing a code needs `signin:approve`, which no device login, root or `write:sensitive` token carries; `approve` and `deny` need `signin:approve` or an unscoped root token of your own user (a root token must type the number, not the password). Mint one from a signed-in session with `levelrail-cli tokens create --name approver --abilities signin:approve --expires-in-days 30` (it must expire within 30 days by default, cannot be an agent token, and is revoked when you change or reset your password or sign out your other sessions), or use the dashboard. `code-login` shows or changes who may sign in with a code (changing it needs root). See [Identity and access](/identity-and-access#sign-in-with-a-code).
- `activity` shows your own account activity feed (needs a token that belongs to you with `read:sensitive`); `preferences` reads or changes your timezone, language, theme, landing page and Gravatar choice; `export` requests the data bundle, waits and saves the zip (mode 0600); `unlink` removes a linked sign-in provider and refuses to remove your last way in; `delete-account` permanently deletes your account after the typed email and your password. See [Your account](/account).
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
levelrail-cli tokens cleanup [--older-than-days N] [--yes] [flags]
```
Every `tokens` subcommand requires a username and password (prompted if omitted), because these routes are session only server side and a bearer token cannot call them. `create` mints a new API token; `--agent` labels it as issued to an AI agent so audit entries record the agent name. `list` never shows a secret. `cleanup` permanently removes revoked tokens that were revoked more than `--older-than-days` days ago (default 90 when the flag is omitted; `0` removes every revoked token and needs `--yes`) and prints how many it removed; unlike the other subcommands it takes a root API token (`--token` or the stored one) instead of a password, and it never touches an active token. The dashboard setting **Delete revoked API tokens after** (Settings, API tokens) does the same on a schedule.

## Domains

Per domain settings take `<app> <domain>`, and the domain must already be one of the app's configured domains. Domain TLS and ingress behavior is covered in [Domains and ingress](/domains-and-ingress).

```
levelrail-cli domains list [flags]
levelrail-cli domains check <app> <domain> [flags]
levelrail-cli domains doctor <app> <domain> [--json]
levelrail-cli domains diagnose <domain> [--fix] [--json]
levelrail-cli domains add <app> <domain>... [--dns auto|off|preview] [--json]
levelrail-cli domains summary [--json]
levelrail-cli domains activity <domain> [--limit N] [--before CURSOR] [--actions PREFIXES] [--json]
levelrail-cli domains certificates [flags]
levelrail-cli domains basic-auth get|set|clear <app> <domain> [flags]
levelrail-cli domains maintenance get|clear <app> <domain> [flags]
levelrail-cli domains maintenance set <app> <domain> [--title T] [--message M] [--until TIME | --for DUR] [--retry-after DUR] [--bypass IPS] [--no-health-bypass] [flags]
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
levelrail-cli domains standalone list [flags]
levelrail-cli domains standalone add <domain> --target TARGET [--confirm] [flags]
levelrail-cli domains standalone update <domain> [--target TARGET] [flags]
levelrail-cli domains standalone remove <domain> [flags]
```

- `standalone` manages [standalone domains](/standalone-domains): domains with no app that proxy to `PORT`, `HOST:PORT`, an `http(s)://` URL, `app:NAME[:PORT]` or `container:NAME:PORT`. Options are `--upstream-tls`, `--insecure-skip-verify`, `--tls-server-name`, `--host-header preserve|upstream|NAME`, `--no-websocket`, `--connect-timeout`, `--read-timeout`, `--strip-prefix` and `--description`. Loopback, private, container and control-port targets need `--confirm`. `update` sends only the flags you pass. Per-domain commands take `_standalone` as the app name, for example `domains waf set _standalone tools.example.com`.

- `list` shows every app's domains in one call. `check` runs a real DNS lookup and reports whether the domain reaches this control plane. `certificates` lists every certificate in certmagic storage, healthy or not.
- `basic-auth set` takes `--username` and `--password`. `maintenance set` serves a fixed maintenance response instead of proxying, without stopping the container. `redirect set --target URL` redirects every request for the domain; maintenance mode takes precedence if both are set.
- `tls-cert` uploads an operator supplied certificate and key; `clear` reverts to automatic issuance and `renew` forces re-issuance of an automatic certificate.
- `waf` configures the opt-in Web Application Firewall and rate limiting. `--mode` defaults to `detect` (matches are logged, never rejected) rather than `block`.
- `error-pages` sets custom HTML for status code 404, 500, 502 or 503; `clear` removes one mapping, or all if `--code` is omitted.
- `dns` reads and writes the real A, AAAA, CNAME, TXT, MX, SRV and CAA records in the domain's zone through whichever DNS-01 provider is configured. `remove` must match an existing record exactly.
- `cloudflare-dns set --cf-api-token TOKEN` and `route53-dns set --aws-access-key-id ID --aws-secret-access-key KEY` configure the ACME DNS-01 credentials needed for wildcard domains. A domain is wildcard eligible by having a leading `*.` label; if both providers are enabled, Cloudflare takes precedence.

## DNS

See [DNS zones and records](/dns).

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

See [Backups and storage](/backups-and-storage).

```
levelrail-cli backups list <database> [flags]
levelrail-cli backups list-all [flags]
levelrail-cli backups trigger <database> [--target ID] [flags]
levelrail-cli backups delete <database> <backup-id> [flags]
levelrail-cli backups download <database> <backup-id> [flags]
levelrail-cli backups restore <database> (--backup ID | --latest) --confirm NAME [flags]
levelrail-cli backups restore-as-new <database> --backup ID --new-name NAME [flags]
levelrail-cli backups restores <database> [flags]
levelrail-cli backups clone-restores <database> [flags]
levelrail-cli backups schedule set <database> --target ID --cron EXPR [flags]
levelrail-cli backups schedule clear <database> [flags]
levelrail-cli backups verify <database> --backup ID [flags]
levelrail-cli backups verifications <database> --backup ID [flags]
levelrail-cli backups health [flags]
levelrail-cli backups protection [--refresh] [flags]
levelrail-cli backups drill run --backup ID [--wait] [flags]
levelrail-cli backups drill list [--app A --volume V | --database D] [--limit N]
levelrail-cli backups drill show <drill-id> [flags]
levelrail-cli backups volumes list <app> <volume> [flags]
levelrail-cli backups volumes policy get <app> <volume> [flags]
levelrail-cli backups volumes policy set <app> <volume> [--retain-daily N --retain-weekly N --retain-monthly N --pre-hook CMD --post-hook CMD --pause]
levelrail-cli backups volumes restore <app> <volume> --backup ID [--new-volume NAME] [--target-app APP] [--node NODE]
```

- `list-all` lists backup history across every database and app volume instance wide.
- `delete` removes one archived backup (destructive). `download` streams a succeeded backup's object to stdout.
- `restore` restores in place and is destructive; `--confirm` takes the database name. `restore-as-new` restores into a brand new database and is non destructive. `restores` and `clone-restores` list the attempt history of each.
- `verify` checks a backup is intact without a live restore; `verifications` lists past attempts.
- `health` shows every backed up resource with its last backup and last verified restore. `protection` shows bucket object lock and versioning.
- `drill run` restores a backup into a scratch resource, validates and destroys it, and exits non zero when it fails. See [Prove your backups work](/prove-your-backups-work).
- `volumes restore` restores into a new volume only, optionally named for another app or on another node.

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

Postgres, MySQL and MariaDB. Recovery covers the time range from when `pitr enable` was called, forward; use `backups restore` for anything older.

```
levelrail-cli pitr enable <database> [flags]
levelrail-cli pitr disable <database> [flags]
levelrail-cli pitr status <database> [flags]
levelrail-cli pitr base-backups list <database> [flags]
levelrail-cli pitr base-backups trigger <database> --target ID [flags]
levelrail-cli pitr restores <database> [flags]
levelrail-cli pitr restore <database> --base-backup ID --target-time RFC3339 [--confirm NAME | --restore-as NEW] [flags]
```

`enable` turns on continuous WAL (Postgres) or binary log (MySQL, MariaDB) archiving going forward; `status` shows whether PITR is enabled and the current recoverable window; `restores` lists point in time restore attempts (base backup, target time, status, error). `restore` overwrites a Postgres database in place and is destructive. For MySQL and MariaDB it needs `--restore-as NEW`, creates that new database and leaves the source untouched.

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
levelrail-cli channels quiet-hours <id> [--start HH:MM --end HH:MM [--timezone Z] | --clear]
```
Valid `--kind` values: `generic`, `slack`, `discord`, `telegram`, `email`, `pushover`, `pagerduty`, `teams`, `resend`, `ntfy`, `gotify`, `mattermost`, `lark`, `rocketchat`, `opsgenie`, `webex`, `googlechat`. `update` fully replaces a channel's configuration. `--notify-device-login` opts a channel into a link-only notice when a CLI login is waiting. `test` sends a real test message; `deliveries` lists a channel's recorded send history, newest first. See [Email notifications](/email-notifications).

## Push subscriptions

```
levelrail-cli push-subscriptions list [flags]
levelrail-cli push-subscriptions revoke <id> [flags]
```
Lists or revokes the browser push subscriptions registered for the caller's own account. There is no `register` subcommand: a subscription's endpoint and keys only come from a real browser. See [Observability](/observability).

```
levelrail-cli push-subscriptions list --json
levelrail-cli push-subscriptions revoke <id>
```

## Alerts

Top-level commands for muting, scheduling and reviewing alerts. The rules themselves are managed under [`apps alerts`](#alerts-connections-and-templates). See [Observability](/observability).

```
levelrail-cli alerts silences list [--all]
levelrail-cli alerts silences create --for DURATION [--rule ID] [--app NAME] [--node NAME] [--kind KIND] [--severity S] [--metric M] [--label k=v] [--reason TEXT]
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

The opt-in public status page. It stays off until `set --enable`. Only public component names, statuses and operator written announcements are published; targets (app names, hostnames, check URLs) stay private. See [Status page](/status-page).

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

Environment variables shared at project, organization or environment scope, plain or secret. See [Projects and organizations](/projects-and-organizations).

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
levelrail-cli registry credentials list|vendors|add|test|delete [flags]
levelrail-cli registry package-registries list|add|test|delete [flags]
```
Private container registry pull credentials. `update` can rotate the password. `test` authenticates against the registry without pulling anything; `repositories` and `tags` browse the external registry. `registry credentials add --vendor ID` pre-fills the host and username for a vendor preset (see `registry credentials vendors`). `registry package-registries` manages private npm, PyPI, Maven, NuGet, Cargo, Go and RubyGems registries mounted into builds as secrets, see [private registries](/private-registries).

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

## Assets

```
levelrail-cli assets buckets list [--project P]
levelrail-cli assets buckets create <name> --target ID [--prefix P] [--public --confirm-name NAME] [--serving direct|cdn] [--public-base-url URL] [--cdn-base-url URL]
levelrail-cli assets buckets show <bucket>
levelrail-cli assets buckets set <bucket> [--public --confirm-name NAME | --private] [--serving direct|cdn] [--referrer-allow H,H] [--quota-bytes N]
levelrail-cli assets buckets probe <bucket> [--origin URL] [--fix-cors]
levelrail-cli assets buckets delete <bucket> [--yes]
levelrail-cli assets usage <bucket> [--egress-gb N]
levelrail-cli assets ls <bucket> [--prefix P] [--cursor C] [--limit N]
levelrail-cli assets upload <bucket> <file> [--key K] [--content-type T]
levelrail-cli assets url <bucket> <key> [--expires D]
levelrail-cli assets rm <bucket> <key>
```
Files served from an S3 compatible storage destination. A `<bucket>` is the bucket name or its id. Buckets start private and Direct; making one public needs the bucket name typed back (`--confirm-name`, or an interactive prompt), never assumed. `upload` sends small files through the control plane and large ones straight to the provider with a presigned URL (the cutoff comes from `LEVELRAIL_ASSETS_DIRECT_MAX_BYTES`, and a 413 also switches routes), then prints the origin and CDN URLs. `buckets probe` checks access, browser upload CORS and public exposure; `--fix-cors` applies the CORS rule. `usage` prices the stored size with list prices and labels the result as an estimate with the price table date. `buckets delete` removes only the connection; objects stay at the provider. All commands take `--json` and `--output`.

## Data export

See [Data export](/data-export).

```
levelrail-cli export destinations list|create|test|status|enable|disable|delete
levelrail-cli export audit [--from T] [--to T] [--format ndjson|csv]
```

## Compliance

```
levelrail-cli compliance evidence create --from DATE --to DATE [--out FILE] [--no-wait]
levelrail-cli compliance evidence list
levelrail-cli compliance evidence download <id> [--out FILE]
levelrail-cli compliance evidence verify <id>
levelrail-cli compliance access-review start [--name N] [--due-days D] [--repeat-days R]
levelrail-cli compliance access-review list
levelrail-cli compliance access-review show <id>
levelrail-cli compliance access-review keep|revoke <id> <item-id>...
levelrail-cli compliance access-review complete <id>
```

## OpenTelemetry export

```
levelrail-cli otel destinations list
levelrail-cli otel destinations add --name N --endpoint URL [--signal metrics|logs] [--app A] [--header K=V] [--label K=V] [--interval S] [--min-level L]
levelrail-cli otel destinations test <id>
levelrail-cli otel destinations remove <id>
```

OTLP/HTTP export of metrics and logs to your own collector. A signal-first view of the OTLP kinds of [Data export](/data-export#otlp-logs); header values are stored encrypted.

## Event webhooks

```
levelrail-cli event-webhooks list
levelrail-cli event-webhooks create --name N --url U [--event apps.*,users.create] [--disabled]
levelrail-cli event-webhooks test <id>
levelrail-cli event-webhooks deliveries <id>
levelrail-cli event-webhooks replay <delivery-id>
levelrail-cli event-webhooks enable|disable <id>
levelrail-cli event-webhooks rotate-secret <id>
levelrail-cli event-webhooks delete <id>
```
Signed outbound event webhooks with a delivery log. `create` and `rotate-secret` print the signing secret once. See [Event webhooks](/event-webhooks).

## Scheduled maintenance

```
levelrail-cli maintenance list
levelrail-cli maintenance create --title T --start TIME (--end TIME | --duration 2h) [--scope all|project|app] [--target NAME] [--freeze] [--notify-before 1h] [--message TEXT]
levelrail-cli maintenance delete <id>
```
One-off planned downtime shown on the dashboard and the public status page. See [Scheduled maintenance](/scheduled-maintenance).

## CDN overview

```
levelrail-cli cdn overview [--window 5m|1h|24h|7d]
```
Fleet requests, 5xx rate, edge cache hit rate and each node's share of traffic over the window. `--json` prints the same object the dashboard CDN page reads.

## Automatic cleanup

```
levelrail-cli janitor status
levelrail-cli janitor run [--dry-run] [--if-needed]
levelrail-cli janitor policy [--enabled=BOOL] [--dry-run=BOOL] [--interval-minutes N] [--disk-free-budget-percent N] [--build-cache-budget-mb N] [--build-cache-max-age-hours N] [--volume-quarantine-days N] [--delete-orphan-volumes=BOOL] [--attempt-retention-days N] [--attempt-keep-per-app N] [--disabled-stages a,b]
```
Disk cleanup with preview and quarantine. See [Automatic cleanup](/janitor).

## Cleanup center

```
levelrail-cli cleanup list [flags]
levelrail-cli cleanup preview ID [--days N] [flags]
levelrail-cli cleanup run ID [--days N] [--dry-run] [--yes] [flags]
levelrail-cli cleanup mode ID off|preview|on [flags]
levelrail-cli cleanup runs [--target ID] [--since DATE] [--format table|json|csv] [flags]
```
`list` shows every cleanup target with its usage, retention, mode and last run. `preview` reports the rows and bytes a run would delete and deletes nothing. `run` deletes data for one target: without `--dry-run` it prints the preview row count first and refuses unless `--yes` is given; `--dry-run` only records what would be deleted, prints `would delete N rows (dry run)` and is audited like any other run. `--days` must be at least 1 and overrides the target's retention for that call only; retention itself is the runtime setting (see `settings runtime`). `mode` sets a target to `off`, `preview` (scheduled runs only record) or `on`. `runs` lists the run history, newest first, and `--format csv` prints it as CSV for a SIEM or spreadsheet. Viewing needs a read token; `preview`, `run` and `mode` need a root API token.

## Search

```
levelrail-cli search <query...> [--kind K] [--limit N] [--json]
levelrail-cli search saved list
levelrail-cli search saved add <name> <query...>
levelrail-cli search saved rm <id>
```
One search over apps, databases, nodes, domains, templates, settings, docs and actions. Scopes (`app:`, `env:`, `tag:`, `node:`, `kind:`) narrow the result, the rest is free text. You only ever see what your token may see, and an action hit appears only when you hold the ability it needs: `levelrail-cli search kind:action restart api`. See [Unified search](/search).

## Logs

See [Log archive](/log-archive).

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

NFS and CIFS network share mounts, the same shares an app's volume mount can reference by name. See [Network shares](/network-shares).

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

Declarative host firewall rules, reconciled onto the control plane's local `ufw`. See [Host firewall](/host-firewall). `exposure`, `restrict` and `unrestrict` are covered in [Exposure audit](/exposure-audit).

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

## Security center

The security score, sessions and devices, and the platform security policy. See [Security center](/security-center).

```
levelrail-cli security posture [flags]
levelrail-cli security sessions list [--user ID] [flags]
levelrail-cli security sessions revoke <id> [--user ID] [flags]
levelrail-cli security sessions revoke-others [--user ID] [flags]
levelrail-cli security policy get [flags]
levelrail-cli security policy set [--approval-scope password_only|all_methods] [--max-token-lifetime-days N] [--warn-unused-days N] [--disable-unused-days N] [flags]
levelrail-cli security hsts status|enable|disable [flags]
levelrail-cli security reachability [--recheck] [--fix ID] [flags]
```

`security reachability` (root) lists what the internet can reach per app and database. `--recheck` re-measures instead of using the cached audit, and `--fix ID` applies a finding's automatic fix and re-checks. See [Exposure report](/security-center#exposure-report).

`security hsts enable` probes every public domain first and refuses until each serves a publicly trusted certificate. HSTS is off by default.

`posture` prints the score for anyone with `read`; the item list needs root. The `sessions` commands act on your own account; `--user` names another account and needs root. A token used for them must belong to a user and hold `write:sensitive` or `signin:approve`. `policy set` needs root and changes only the flags you pass.

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

Experimental: requires `APP_EXPERIMENTAL=iac`. See [Platform as code](/platform-as-code) for the document format, secrets handling, prune rules and CI use.

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
Writes live state as stable resource files, never containing secret values. Secret looking values become <span v-pre>`${{ env.NAME }}`</span> placeholders; supply them at apply time with `--var`, `--var-file` or `--allow-env`. Callers without `read:sensitive` always get placeholders for every env value.

## Nodes

See [Multi-node](/multi-node).

```
levelrail-cli nodes list [flags]
levelrail-cli nodes get <id> [flags]
levelrail-cli nodes delete <id> [flags]
levelrail-cli nodes join-token [flags]
levelrail-cli nodes reenroll-token <id> [flags]
levelrail-cli nodes revoke-cert <id> [flags]
levelrail-cli nodes security <id> [--rejected] [--limit N] [flags]
levelrail-cli nodes cert renew <id> [flags]
levelrail-cli nodes cert drill <id> [flags]
levelrail-cli nodes cordon <id> [flags]
levelrail-cli nodes uncordon <id> [flags]
levelrail-cli nodes add [--name NAME] [--mesh] [flags]
levelrail-cli nodes drain <id> [--target NODE-ID] [--wait] [flags]
levelrail-cli nodes drains <id> [flags]
levelrail-cli nodes failure-policy get|set <id> [--mode off|notify|auto] [--grace SECONDS] [flags]
levelrail-cli nodes lost <id> [--dry-run] [--yes] [flags]
levelrail-cli nodes failure-resolve <id> app|database NAME --wait|--restore [--target NODE-ID] [flags]
levelrail-cli nodes move app|database NAME [--to NODE-ID] [--wait] [flags]
levelrail-cli nodes labels <id> [key=value ...] [flags]
levelrail-cli nodes placement app|database NAME [key=value ...] [flags]
levelrail-cli nodes connectivity [flags]
levelrail-cli nodes workloads <id> --accepts-app=BOOL --accepts-build=BOOL [flags]
levelrail-cli nodes health <id> [flags]
levelrail-cli nodes patch-status <id> [flags]
levelrail-cli nodes events <id> [--limit N] [flags]
levelrail-cli nodes metrics <id> --metric NAME [flags]
levelrail-cli nodes resource-usage [flags]
levelrail-cli nodes capacity-forecast <id> [flags]
levelrail-cli nodes capacity <node> [flags]
levelrail-cli nodes cleanup <node> --dry-run|--yes [flags]
levelrail-cli nodes mesh [flags]
levelrail-cli nodes rotate-key <id> [flags]
levelrail-cli nodes rejoin-mesh <id> [flags]
levelrail-cli nodes topology [flags]
levelrail-cli nodes traffic [flags]
levelrail-cli nodes providers list|connect|test|disconnect|set-credential [flags]
levelrail-cli nodes provision --provider P --region R --size S --name NAME [flags]
levelrail-cli nodes provisions list|show <id> [flags]
levelrail-cli nodes ssh-provision --host HOST --user USER (--key-file FILE | --password PASS) --name NAME [--host-key-fingerprint SHA256:... | --insecure-skip-host-key-check] [flags]
levelrail-cli nodes ssh-provisions list|show <id> [flags]
```

- `delete` fails while the node still has placements; drain it first. `join-token` mints a one-time enrollment token, shown once. `reenroll-token` mints a one-time token that re-issues a node's agent certificate, keeping its identity, and prints the command to run on the node. `revoke-cert` revokes the certificate and closes the node's session; only a re-enroll token brings it back. `security` prints how the control plane verifies the node (CA and certificate fingerprints, renewal schedule and last failure, 24 hour check counts) and its recent security events; `cert renew` asks a connected node to renew its certificate now. `cert drill` rotates one node's certificate end to end (preflight, renewal request, new certificate, node still online), prints each step and exits non-zero on a failure. The old certificate is never revoked, so a failed drill changes nothing; wait time is `APP_NODE_CERT_DRILL_TIMEOUT` (30s).
- `cordon` marks a node unschedulable without evacuating it; `uncordon` reverses that; `drain` moves every service and database off a node.
- `list` shows each node's certificate state and days left (CERT) and agent version (AGENT); `get` adds expiry, last renewal, key origin, platform and commit.
- `resource-usage` shows every node's latest CPU, memory and disk usage plus a fleet rollup; `capacity-forecast` projects disk and memory usage forward, roughly how many days until full.
- `mesh` shows the live WireGuard mesh state and peers; `rotate-key` rotates a node's WireGuard key; `rejoin-mesh` forces an immediate resync for a stuck peer.
- `providers` manages cloud provider credentials (Hetzner, DigitalOcean) and `provision` creates a server at a provider and enrolls it as a node; `ssh-provision` adopts a machine you already have over SSH. `provisions` and `ssh-provisions` track either through to enrollment. See [Multi-cloud provisioning](/multi-cloud-provisioning).
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

Compares the running version with the latest release, runs the preflight checks (release signature, Docker Engine, free disk, backup), takes a control plane backup and prints the command that upgrades. It never upgrades by itself unless you pass `--apply`. See [Installing](/installing#check-first-then-upgrade).

```
levelrail-cli upgrade --plan [--version TAG]
levelrail-cli upgrade --apply [--version TAG] [--ack-breaking ID,...] [--no-wait]
levelrail-cli upgrade --attempts
levelrail-cli upgrade status
levelrail-cli upgrade backup-now
levelrail-cli updates restore-points list
levelrail-cli updates restore-points restore <id> --yes
levelrail-cli upgrade schedule (--at TIME | --window) [--version TAG] [--timezone TZ] [--ack-breaking ID,...] [--ack-major] [--replace]
levelrail-cli upgrade schedule show
levelrail-cli upgrade schedule cancel
levelrail-cli upgrade auto get
levelrail-cli upgrade auto set [--mode off|patch|any] [--window-cron EXPR] [--window-duration 2h] [--timezone TZ] [--min-age-days N] [--notify ID,...] [--pause | --resume]
```

`backup-now` takes a control plane backup and re-runs the preflight. `schedule` queues the upgrade for a time or the next maintenance window; the preflight re-runs at that time and nothing is applied if it fails. `auto` reads and sets the automatic upgrade policy, which is off by default; `set` changes only the flags you pass. All of these need a root token. See [Upgrade safely](/upgrade-safely).

`--plan` shows the breaking changes between the running version and the target (default: latest on the channel). `--apply` asks the control plane to run the safe self-update on its host: verified download, snapshot, migration dry run, swap, health check and automatic rollback. Breaking changes flagged as needing acknowledgement are refused until you list their ids with `--ack-breaking`. It follows the attempt to its outcome unless `--no-wait`, and exits non-zero when the upgrade was rolled back or failed. `--attempts` lists every attempt with its step timeline. See [Upgrade safely](/upgrade-safely).

## System restart {#system-restart}

```
levelrail-cli system restart [--wait] [--yes] [--timeout 120s] [flags]
levelrail-cli system restart status [flags]
```

Restarts the control plane process through its supervisor, for example after enabling the mesh. It asks for confirmation unless `--yes`; without a terminal, `--yes` is required. With `--wait` it polls until the control plane answers from a new process (its `boot_id` changes), tolerating connection errors while it is down, and gives up after `--timeout`. `status` prints the current `boot_id`, whether restart is supported and whether one is pending.

If no process supervisor was detected, the command prints the reason and the command to run on the server (for example `sudo systemctl restart levelrail`) and exits non-zero. The API is `GET` and `POST /api/v1/system/restart`.

## System health {#system-health}

```
levelrail-cli system health [--fix wal_checkpoint|db_optimize|reset_slow_log|telemetry_prune|reset_reconcile_guard|integrity_check|integrity_restore] [flags]
```

Prints goroutines, heap, database latency and write queue, write ahead log size, reconcile pass timing and live stream use, with a finding for each value over its threshold. `--fix` applies one of the typed fixes and prints the result; it needs a root token. The API is `GET /api/v1/system/control-plane-health` and `POST /api/v1/system/control-plane-health/fix`. See [Control plane health](/control-plane-health).

## System integrity {#system-integrity}

```
levelrail-cli system integrity [--check [--full]] [--restore NAME|latest] [--cancel-restore] [flags]
```

Prints whether the control plane database passed its last integrity check and lists the snapshots that can be restored. `--check` runs a quick check now, `--full` the full `integrity_check`. `--restore` verifies a snapshot and stages it to replace the database at the next start (`latest` picks the newest one that verifies), and `--cancel-restore` removes a staged restore. Staging needs a root token. The API is `GET /api/v1/system/integrity`, `POST /api/v1/system/integrity/check`, and `POST` or `DELETE /api/v1/system/integrity/restore`. See [Resilience](/resilience#database-integrity-and-guided-restore).

## System networks {#system-networks}

```
levelrail-cli system networks [--orphans] [flags]
levelrail-cli system networks prune --dry-run|--yes [flags]
```

Lists the per-app Docker networks with owner, subnet, attached container count and whether the owning app still exists, plus how full the `APP_NETWORK_POOL` block is. `prune` removes only orphaned networks that are past `APP_NETWORK_GC_GRACE`, empty, and not owned by another instance; pass exactly one of `--dry-run` or `--yes`. The API is `GET /api/v1/system/networks` and `POST /api/v1/system/networks/prune`. See [Troubleshooting](/troubleshooting#app-network-pool).

## Readiness

```
levelrail-cli readiness [--domain example.com] [flags]
```

Checks whether the server is ready: who holds ports 80 and 443, Docker version and rootless mode, exposed Docker API, disk, memory, firewall and the DNS of the domain. Prints the recommended install mode (own ports or behind your existing proxy) and the exact next step. Exits 1 when a check fails. Before anything is installed use `install.sh --check` instead, see [Installing](/installing#check-the-server-first).

## Version

```
levelrail-cli version [--server] [flags]
```
Prints this CLI's version, commit and build date. It runs offline: no server, login or config is needed. With `--server` it also asks the control plane for its running version, whether that matches the CLI, and whether a newer release has been published to the project's GitHub repository. `--server` fails with `could not reach the server at <url>` when the control plane is unreachable; the plain command still exits 0. `--json` returns both parts.

## Changelog

```
levelrail-cli changelog [--limit N] [flags]
```
Prints recent release notes, newest first, from the running control plane's own CHANGELOG.md. Without `--limit` the server default applies. The dashboard shows the same notes in the [What's new panel](/whats-new-panel).

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

See [Agents](/agents).

## API docs

```
levelrail-cli api-docs [flags]
```
Prints every HTTP route registered on the control plane: method, path, required ability and resource group, followed by a count of routes and how many have a worked request and response example. It is the same data as Settings, API explorer in the dashboard, without the interactive try-it. Supports `--json`, `--output` and `--query`. See [API explorer](/api-explorer).

```
levelrail-cli api-docs --query "routes[?method=='GET'].path"
```

## Deploy link

```
levelrail-cli deploy-link --image REF --name NAME --port PORT [--env K1,K2] [--health /path] [--server URL] [--launcher URL] [flags]
```
Prints a "Deploy with" link and the README Markdown for it, after validating the image reference, DNS label name, port range, uppercase env names and health path. Env values are never part of a link. Without `--launcher`, the launcher is `APP_BRAND_DOCS_URL` plus `/deploy.html`. `--server` also prints the direct `https://<server>/apps/new?deploy=1&...` link. Supports `--json`, `--output` and `--query`. See [Deploy links and badge](/deploy-link).

```
levelrail-cli deploy-link --image traefik/whoami:latest --name whoami --port 80 --health /health
```

```
levelrail-cli deploy-link --repo URL [--branch B] [--name N] [--port P] [--profile P] [--api-url URL] [--open] [flags]
levelrail-cli deploy-link --template ID [--name N] [--port P] [--profile P] [--api-url URL] [--open] [flags]
```
Prints a link to the current profile's instance (`/deploy?repo=...`) plus README Markdown and HTML button snippets. The repo must be an https or ssh git URL without a username or password. `--open` opens the link in the browser. Nothing deploys from the link. See [Deploy button](/deploy-button#git-repositories-and-templates).

```
levelrail-cli deploy-link --repo https://github.com/you/app --branch main --name app --open
```

## AI assistant

Experimental: requires `APP_EXPERIMENTAL=ai-chat`, and a provider key saved with `settings ai-assistant providers set` and a model chosen with `settings ai-assistant use`. The assistant is read and suggest only: a tool call that would change state pauses for an explicit confirmation. See [In-app AI assistant chat](/ai-assistant-chat).

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

See [AI control](/ai-control).

## Audit log

```
levelrail-cli audit-log [flags]
levelrail-cli audit-purge [flags]
```

`audit-log` lists every recorded write, deploy and root tier request plus automatic certificate renewals, newest first. Read only requests are not recorded. It needs an admin or root scoped token. Flags: `--limit N`, `--before RFC3339`, `--path`, `--method`, `--client-kind cli|dashboard|mcp|api`, `--agent <name>` (entries made with a token labeled with that agent name), `--search <text>` (case-insensitive substring across actor, ability, method, path and remote address), `--resource domain:<name>|zone:<name>|app:<name>` (one resource's own trail), `--actions <prefixes>` (comma separated action prefixes such as `dns_record.,proxy_route.,domain.`), `--failed` (status 400 or higher), `--format csv` with `--output-file FILE`. `--agent`, `--search` and `--failed` are applied server side and carry into csv exports.

`audit-purge` deletes every entry older than the retention window (`APP_AUDIT_LOG_RETENTION_DAYS`, default 90 days) right now instead of waiting for the automatic sweep. Security-family entries are kept for `APP_AUDIT_LOG_SECURITY_RETENTION_DAYS` (default 365). It needs an admin or root scoped token.

`audit-verify` walks the audit log's hash chain and signed checkpoints and prints any entry that was edited, removed or cut off the end. It exits 1 when a check fails and needs a root token.

`tokens create` and `auth login` default to a 90 day lifetime. A token that never expires needs `tokens create --no-expiry` from a root caller, and the security policy's maximum lifetime still overrides it.

## Attention

```
levelrail-cli attention [flags]
```

Lists everything that needs attention right now: failing apps, failed deploys from the last 24 hours, low disk space (warn under 10 percent free, critical under 5), offline nodes, expired or expiring certificates, and doctor warnings or failures, critical first. It is the CLI side of the dashboard's Status page (`/status`). Exit code is 1 if any item is critical, 0 otherwise, so it works as a script gate. Supports `--json`, `--output json|table|text`, and `--query`.

## TUI

```
levelrail-cli tui [--read-only] [--refresh DURATION] [flags]
```

Opens an interactive terminal UI over the same API: live apps, databases, deploys, nodes, domains, alerts and backups, a log tail, and confirm-gated restart, deploy, rollback, stop and start. Needs a TTY. See [Terminal UI](/cli-tui) for the key map.

## Go live

```
levelrail-cli go-live <app> [domain] [--wait] [--timeout 5m] [flags]
```

Prints the [go live checklist](/go-live) for an app and domain (default: the app's only domain): app health, port, host firewall, DNS, certificate and the final HTTPS check, each with its status and next action. With `--wait` it polls and exits 7 until the domain is live or `--timeout` passes. Supports `--json`.

## Doctor

```
levelrail-cli doctor [flags]
```

Runs a local preflight health check: Docker daemon reachability, disk space and write latency, write access on the data directory, port 80 and 443 availability for the embedded ingress, control plane database reachability, RAM and CPU against the recommended minimums, firewall status, outbound network reachability (public IP, external port reachability, ACME, clock skew, the agent advertise host, image registries), and GPU attach checks. Exit code is 0 if every check is ok or warn, 1 if any check fails.

## Resilience

```
levelrail-cli resilience check [--simulate] [--fix] [flags]
```

Shows whether apps keep serving if the control plane is down: route snapshot, certificate cache, and per node agent link, each pass, warn, fail or unknown. `--simulate` is a dry run that verifies the saved snapshot would let ingress restart without the control plane and stops nothing. `--fix` rewrites the route snapshot from the applied config. Supports `--json` and `--output json`.

## Setup

```
levelrail-cli setup status [flags]
levelrail-cli setup next [flags]
levelrail-cli setup skip <step> [flags]
levelrail-cli setup undo <step> [flags]
levelrail-cli setup reset [flags]
```

The dashboard's Get started checklist from a terminal. `status` prints every step with its state, time estimate and note, plus the "ready for production" percentage. `next` prints the one step to do next and where to do it. `skip` and `undo` record or clear an operator override for a step id such as `teammate`; `reset` clears every override and shows the guide again. The three writes need a root scoped token. Steps and their rules: [Getting started](/getting-started#finish-setting-up-for-real-use). Supports `--json`, `--output json|table|text`, and `--query`.

## Proxy

```
levelrail-cli proxy [--domain D | --app NAME] [--proxy traefik|nginx|caddy] [--verify] [flags]
levelrail-cli proxy setup [--confirm] [--provider traefik|nginx|caddy|haproxy|auto] [--dynamic-dir DIR] [--cert-dir DIR] [flags]
levelrail-cli proxy status [flags]
levelrail-cli proxy apply [flags]
levelrail-cli proxy verify [--domain D] [flags]
levelrail-cli proxy disable [flags]
```

Without a subcommand: shows which container publishes ports 80 and 443 and the configuration to paste into that proxy, for the dashboard (`--domain`) or an app's domain (`--app`, upstream is the ingress HTTP port). `setup` detects a Traefik (Coolify's included) and, with `--confirm`, turns on `tls_terminated_upstream`, saves the detected entrypoints, certificate resolver, directory and upstream host, writes one route file per domain and verifies each over HTTPS; without `--confirm` it is a dry run. It exits with an API error and the precise gap (`detection_incomplete`, `upstream_unreachable` with the drop-in fix, `not_writable`) when it cannot proceed. `status` prints detection, per-domain state (`missing`, `written`, `stale`, `error`), certificate issuer and expiry, and the checklist. `apply` rewrites the files now, `verify` probes again, `disable` removes the files it wrote. Writes need a root scoped token. Details: [Run behind an existing proxy](/behind-an-existing-proxy#one-step-setup-traefik-including-coolify).

`--provider` picks the proxy (`auto`, the default, uses the one detected). nginx is managed: `setup --confirm` writes one `.conf` per domain, validates it with `nginx -t` and rolls back on failure. Caddy and HAProxy only render a snippet to apply by hand.

## Containers

```
levelrail-cli containers [list] [flags]
levelrail-cli containers stop <name> [flags]
levelrail-cli containers remove <name> [flags]
levelrail-cli containers claim <name> [--as app-name] [flags]
levelrail-cli containers orphans [flags]
levelrail-cli containers reap [--dry-run] [flags]
```

`list` shows every container on this node, managed or not. `stop`, `remove` and `claim` only act on a container the platform does not already manage (409 otherwise); for a managed app use `apps stop`, `apps start`, `apps restart` or `apps delete`. `claim` adopts an orphaned container into a new app. `orphans` lists leftover containers, volumes and certificates that no app or database accounts for, with where each stands against its grace period. `reap` runs one removal pass now (`--dry-run` reports without removing; exit 1 if anything could not be removed). Details and the `APP_ORPHAN_*` settings are in [Delete and clean up](/deploying-apps#delete-and-clean-up).

## System address

```
levelrail-cli system address [--domain NAME] [flags]
levelrail-cli system address --ipv4 IP [--ipv6 IP]
levelrail-cli system address --clear
```

Shows this server's public IPv4 and IPv6, where they came from, and a note when the server is behind NAT or a TLS-terminating proxy. `--domain` adds the exact DNS records to create for that name. `--ipv4` and `--ipv6` set a manual override (root), `--clear` goes back to detection. See [Standalone domains](/standalone-domains#server-address-and-dns-records).

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
levelrail-cli control-plane-backups restore-plan
levelrail-cli control-plane-backups keys generate [--out FILE] [--hybrid]
levelrail-cli control-plane-backups help-dr
```

Snapshots of the control plane's own database, stored under `<data dir>/control-plane-backups/`. `create` takes one now (manual snapshots are never auto-deleted), `download` saves one to `--out FILE` (or raw bytes to stdout), `verify` re-checks the checksum, SQLite integrity and schema version without restoring (exit 1 if any check fails), `delete` removes one. Snapshots never contain the master key. Restore is an offline server command, `levelrail restore-db <file>`; see [Control plane backup and restore](/control-plane-backup).

The `--offbox`, `schedule`, `run-now`, `drill`, `escrow` and `keys` subcommands drive encrypted off-box backups. `keys generate` makes an age key pair on your machine (private key to a `0600` file, public key to stdout). `schedule set` changes only the flags you pass (`--enable`, `--disable`, `--destination`, `--recipient`, `--schedule`, `--drill-schedule`, `--retain-daily`, `--retain-weekly`, `--retain-monthly`, `--escrow-destination`). `run-now` and `drill run` wait for the run and exit 1 if it failed; `drill status` exits 1 when the last drill failed or none has run. `escrow` writes the master key and agent CA key encrypted to your recipients (never uploaded unless `--upload`, and never to the backup bucket). `restore-plan` prints the ordered recovery steps with the newest complete backup and the exact restore command filled in. `help-dr` prints a static runbook. Restore an off-box backup on the server with `levelrail restore --from <s3://... | file> --identity FILE [--dry-run]`. See [Disaster recovery](/disaster-recovery).

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

`--role` is a curated preset (`admin`, `operator`, `viewer`, `guest`, or a custom role by name; `users roles` lists the curated presets and `roles list` lists every stored role); `--abilities` is a comma separated list from `read`, `read:sensitive`, `write`, `write:sensitive`, `deploy`, `root`. `users role set` changes a user's role; `users grants` manages which environments a guest can see. `invites create` invites a teammate by email; `invites list` and `invites revoke` manage pending invites. See [Access control](/access-control) for details.

## Roles

```
levelrail-cli roles list [flags]
levelrail-cli roles create <name> --abilities LIST [--visibility all|granted] [--description TEXT] [flags]
levelrail-cli roles update <role> [--name N] [--abilities LIST] [--visibility V] [--description T] [flags]
levelrail-cli roles delete <role> [flags]
```

Custom roles are presets over an ability list. Updating a role applies the new abilities to every user holding it, immediately. A role cannot be deleted if any user holds it. See [Access control](/access-control).

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

`update` replaces the policy's name, description and document. `attachments` lists a policy's attached principals. `templates` shows built-in policy templates and `apply` instantiates one, optionally attaching it to a principal. See [Access control](/access-control).

## Environments

```
levelrail-cli environments list [flags]
levelrail-cli environments create --name NAME [--kind dev|test|uat|production|custom] [--protected] [--sort-order N] [flags]
levelrail-cli environments update <id> [--name N] [--kind K] [--protected[=false]] [--sort-order N] [flags]
levelrail-cli environments delete <id> [--move-to ID] [flags]
levelrail-cli apps move-env <app> <environment> [--confirm] [flags]
levelrail-cli databases move-env <db> <environment> [--confirm] [flags]
```

Global environments exist across the entire instance (Development, Test, UAT, Production). Kinds control which environment a resource has and what policies apply. Moving into or out of a protected environment requires confirmation and approval from another user. See [Environments](/environments).

## Secrets

```
levelrail-cli secrets generate-master-key --out PATH
levelrail-cli secrets rotate-master-key --new-key-file PATH [flags]
levelrail-cli secrets binding-status [flags]
levelrail-cli secrets rebind [flags]
```

`generate-master-key` writes a new master key to a file (mode 0600, never overwrites), locally, without contacting the control plane. `rotate-master-key` re-wraps every stored data encryption key under a new master key in one atomic step, live, then binds any legacy values. `binding-status` counts stored secret values not yet bound to their slot, and `rebind` binds them (safe to rerun). Read [Master key rotation](/master-key-rotation) before running these in production.

## Migrate

```
levelrail-cli migrate coolify --url URL --token TOKEN [flags]
levelrail-cli migrate dokploy --url URL --token TOKEN [flags]
levelrail-cli migrate caprover --url URL --token TOKEN [flags]
```

One way migration from a live instance. By default it writes `app.yaml` files plus a migration report into `--out-dir` (default `./migrated`); `--apply` creates the apps directly on a target instance instead, using `--target-token`, `--target-api-url` and `--target-profile`. `--include-secret-values` also fetches real env var values. For Coolify, `--token` is an API bearer token (with the `read:sensitive` ability if you want secret values); for Dokploy it is an API key; for CapRover it is the login password.

## Stack templates {#stack-templates}

```
levelrail-cli stack templates [--json]
levelrail-cli stack init <template> [--dir DIR] [--set key=value]... [--force] [--json]
```

`stack templates` lists the bundled starter stacks: `web-app-postgres`, `worker-queue`, `static-site-cdn`, `agent-app` and `preview-env`. `stack init` copies one into `DIR` (default `./<name>`) and fills in the variables `name`, `domain` and `image`. It refuses a non-empty directory unless `--force` is set, and writes secret names only (`SECRETS.md`), never values. Both run offline. See [Stacks and GitOps](/stacks#start-from-a-template).

## Import

```
levelrail-cli import <repo-url|image> [--deploy] [--name NAME] [--ref REF] [--port N] [--env KEY=VALUE]... [flags]
levelrail-cli import <repo-url> --analyze|--apply [--base-directory DIR] [--dockerfile PATH] [--build-type TYPE] [--no-database] [flags]
levelrail-cli import -f compose.yaml|Dockerfile [flags]
levelrail-cli import --docker-run "docker run -p 80:80 nginx:1" [flags]
levelrail-cli import platform coolify|dokploy|caprover|heroku|vercel|render --url URL [flags]
levelrail-cli import platform heroku|vercel|render|coolify|dokploy|caprover --emit stack --out DIR [--stack NAME] [--image-repo REPO] [--db-version postgres=16]
levelrail-cli import cutover plan|run|status|rollback|confirm-dns --session ID --app NAME [flags]
levelrail-cli import config [dir] [--out FILE] [--strict] [--json]
levelrail-cli import config [dir] --emit stack --out DIR [--stack NAME] [--image-repo REPO] [--service S] [--db-version postgres=16]
levelrail-cli import takeover plan|run|status|rollback [flags]
```

`import` classifies its input (a GitHub, GitLab, Gitea or Bitbucket repo URL, a `docker run` command, an image reference, a docker-compose.yml or a Dockerfile) and prints a deployment plan. Nothing is created unless `--deploy` is given. `--file -` reads from stdin. `--env KEY=VALUE` is repeatable, and a required variable with no value blocks `--deploy`.

`import <repo-url> --analyze` prints what the New app wizard would suggest (stack, build, port, health path, env variable names, databases) with the evidence for each, and creates nothing. `--apply` creates the suggested databases, the app, the connections, the repository link and the first build; `--base-directory`, `--dockerfile`, `--build-type`, `--name`, `--port` and `--env` override suggestions. See [New app wizard](/new-app-wizard).

`import platform` reads apps, databases and settings from another platform (read only) and creates them here. Use `--dry-run` first. Databases and volumes are created empty; data is not migrated. Re-running skips what was already imported. The source token is read from `APP_IMPORT_SOURCE_TOKEN`, `--token-stdin` or `--token` (which lands in shell history). Other flags: `--only`, `--collision suffix|skip`, `--insecure-tls`, `--allow-private`, `--allow-loopback`. See [migrating from Coolify, Dokploy or CapRover](/migrating-from-coolify-dokploy-and-caprover).

`import cutover` moves one staged app's traffic in a reversible flow. `plan` prints the readiness checklist, `run --dry-run` exercises everything except the switch, `run --confirm NAME` switches and verifies (rolling back on its own if verification fails), `status` shows the timeline, and `rollback` restores the previous DNS value and stops the staged app. Other flags: `--run ID`, `--accept-warnings`, `--no-wait`, `--wait DURATION`, `--json`. See [guided cutover](/migrating-from-coolify-dokploy-and-caprover#guided-cutover-of-one-staged-app).

`import config` reads `Procfile`, `app.json` and `vercel.json` from a directory (default: the current one) and prints a proposed `app.yaml`. Findings go to stderr, blockers first, each with a fix. It is offline and read-only, needs no token and creates nothing. `--out FILE` writes the spec to a file, `--json` prints `{detected, spec, findings, nextSteps}`; the human output ends with NEXT STEPS, the exact `apps create --file ... --service ... --secret KEY=VALUE` and `databases create` commands. A Heroku plus Vercel directory keeps both services when their names clash: the Heroku one keeps the name and the Vercel one gets a `-vercel` suffix (then `-vercel-2` and so on) with its own cron jobs, plus a warn finding. `apps create --file` refuses a spec with `databases:` or `from:` env references instead of dropping them, and `--strict` exits 2 when any finding is a blocker (otherwise it exits 0). `LEVELRAIL_IMPORT_WEB_PORT` (default 8080) and `LEVELRAIL_IMPORT_NODE_PORT` (default 3000) set the proposed ports.

`--emit stack` on `import config` and `import platform` writes reviewable documents instead of applying: a `Stack`, a `Project` (and with `--environment` an `Environment`), `Database`, `App` and `Domain` documents, a `SECRETS.md` checklist of secret names (never values) and a `MIGRATION.md` of what the documents cannot carry. `--out DIR` is required and an existing file is only overwritten with `--force`. The output is sorted, so the same source gives the same bytes, and it loads with `stack plan`. `--stack` names the stack (default `PLATFORM-import`, or the directory name for `import config`), `--project` defaults to the stack name. Apply runs prebuilt images only, so an app with no image takes `--image-repo REGISTRY/NAMESPACE` and is named `REGISTRY/NAMESPACE/APP:latest`; without it the emit fails and says so. `import config` also takes `--service S` (comma separated, as `apps create --service`) and `--db-version postgres=16,redis=7.4`, because the config files name no database version; `import platform` reads the source directly and needs no control plane, and takes the same `--db-version`. Database connection variables and `from:` references are written as secret names in `SECRETS.md` until the schema can reference a database. See [Stacks and GitOps](/stacks#migrate-into-a-stack).

`import takeover` moves this server's ports 80 and 443 from the old platform's proxy to the built-in ingress. `plan` and `run --dry-run` print the preflight and steps and change nothing, `run --confirm takeover` does it and waits through the restart, `status` shows the latest run and `rollback` restores the previous addresses and starts the old proxy again. Other flags: `--accept-unmanaged`, `--run ID`, `--reason TEXT`, `--no-wait`, `--wait DURATION`, `--json`. Root only. See [Migrate on the same server](/migrate-same-server).

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
levelrail-cli settings email providers list [flags]
levelrail-cli settings email providers add --backend NAME [--name N] [--from ADDR] [--secret-stdin] [flags]
levelrail-cli settings email providers set <id> [flags]
levelrail-cli settings email providers rm <id> [flags]
levelrail-cli settings email providers test <id> [--to ADDR] [flags]
levelrail-cli settings email providers promote <id> [flags]
levelrail-cli settings email providers reveal <id> <field> --yes-print [flags]
levelrail-cli settings email branding get|set|preview [flags]
levelrail-cli settings ingress get [flags]
levelrail-cli settings ingress set [--primary-domain D] [--acme-enabled] [--public-https-port N] [--tls-terminated-upstream] [--apps-base-domain HOST] [--dns-cname-target HOST] [--dns-ttl SECONDS] [--dns-proxied] [flags]
levelrail-cli settings ingress https status [flags]
levelrail-cli settings ingress https enable [--email EMAIL] [--staging] [--wait 2m] [flags]
levelrail-cli settings edge get [flags]
levelrail-cli settings edge set [--trusted-proxies LIST] [--max-body SIZE] [--max-header SIZE] [--read-header-timeout DUR] [--idle-timeout DUR] [flags]
levelrail-cli settings edge fix [flags]
levelrail-cli settings dashboard-url get [flags]
levelrail-cli settings dashboard-url set --url URL [flags]
levelrail-cli settings dashboard-url fix [flags]
levelrail-cli settings runtime list [flags]
levelrail-cli settings runtime get KEY [flags]
levelrail-cli settings runtime set KEY VALUE [--confirm-weakening] [flags]
levelrail-cli settings runtime reset KEY [flags]
levelrail-cli settings runtime apply [flags]
levelrail-cli settings updates get [flags]
levelrail-cli settings updates set --channel CHANNEL [--auto-update] [flags]
levelrail-cli settings deploy-freeze show [flags]
levelrail-cli settings deploy-freeze set --cron EXPR --duration D [flags]
levelrail-cli settings deploy-freeze clear [flags]
levelrail-cli settings ai-assistant get|set|clear [flags]
```

- `oauth set <provider>` enables, configures or disables one sign in provider; `<provider>` is `google`, `github` or `oidc`.
- `email get|set` reads and writes the primary provider plus the sender identity and is kept for scripts that predate providers. `email providers` manages one primary and at most one backup: the first provider added is the primary, the second the backup, and sending falls over to the backup when the primary fails before a message is accepted. The credential is read from stdin (`--secret-stdin`) or a hidden prompt, never from arguments; `set` keeps the stored one unless you supply another. `test <id>` sends through that provider alone, `promote <id>` swaps the roles, and `rm` on the primary is refused while a backup exists.
- `ingress get` shows the primary domain, ACME settings, the automatic hostname toggle and the detected public address. `ingress set` changes them (`--primary-domain`, `--acme-enabled`, `--acme-email`, `--acme-directory-url`, `--fallback-domains=false`, `--hsts-enabled`, `--apps-base-domain`, `--dns-cname-target`, `--dns-ttl`, `--dns-proxied`); flags you leave out keep their current value.
- `ingress https status` shows whether the dashboard's free sslip.io HTTPS is off, pending, issued or failed. `ingress https enable` points the dashboard at `<dashed-ip>.sslip.io` and issues a real Let's Encrypt certificate for it, with no DNS setup. `--email` defaults to your admin account email. Plain HTTP sign-in stops once the https dashboard URL is saved.
- `edge get|set|fix` shows and changes request limits and the trusted proxy list; `fix` detects the proxy in front of this server and trusts it. `levelrail-cli ingress-limits show|set|clear APP` is the per-app request body cap. See [Domains and ingress](/domains-and-ingress#edge-limits-client-ips-and-failover).
- `dashboard-url set` sets the public dashboard URL; once it is `https://`, sign in over plain HTTP is refused (`--url ""` clears it).
- `dashboard-url fix` switches the URL to `https://` on the primary domain once it answers over https with a valid certificate. The same Fix button sits in Settings, General (Settings health) and Security. CLI and MCP gating of beta features follows the Settings switches (`APP_EXPERIMENTAL` stays an override).
- `updates` configures the release channel and auto update checking. `deploy-freeze` manages fleet wide freeze windows; per app windows are under `apps freeze`.
- `ai-assistant` is experimental (`APP_EXPERIMENTAL=ai-chat`): `get` shows the active model, the fallback chain and the limits (keys are never returned), `providers list|set|test|remove|models` manages provider keys, `use <provider> <model>` picks the active model, `fallback set` sets the fallback chain, and `usage` shows token use.

## Git integrations

```
levelrail-cli git-providers [list] [flags]
levelrail-cli git-providers validate <github|gitlab|gitea|bitbucket> [flags]
levelrail-cli git-providers repos <github|gitlab|gitea|bitbucket> [--connection ID] [--search Q] [--owner O] [--archived MODE] [--forks MODE] [--limit N] [--offset N] [--all] [--refresh] [flags]
levelrail-cli github-app status|disconnect|repos [flags]
levelrail-cli github-app branches <owner> <repo> [flags]
levelrail-cli github-app use-as-source <owner> <repo> --app-name NAME [flags]
levelrail-cli github-app register-url [--owner ORG] [--public] [--instance-url URL] [--name NAME] [flags]
levelrail-cli github-app installations list|add [flags]
levelrail-cli github-app installations remove <id> [flags]
levelrail-cli git-providers connections [--provider P] [flags]
levelrail-cli git-providers add <gitlab|bitbucket|bitbucket_server|gitea|forgejo> [--instance-url URL] [--client-id ID] [--client-secret S] [--pat TOKEN] [--label L] [--reuse ID] [flags]
levelrail-cli git-providers rename <id> <label> [flags]
levelrail-cli git-providers default <id> [flags]
levelrail-cli git-providers disconnect --id ID [flags]
levelrail-cli git-providers branches --id ID --repo REPO [flags]
levelrail-cli git-providers use-as-source --id ID --repo REPO --app-name NAME [flags]
```

- `git-providers` shows connection status and capabilities (list branches, register a webhook, authenticated clone) for GitHub, GitLab, Bitbucket and Gitea in one call.
- `git-providers repos` lists every repository a provider connection can see from the control plane's cached list, with server-side search, owner, archived and fork filters. `--all` pages through every match, `--refresh` re-lists from the provider first.
- Connecting a provider is dashboard only, because it is a browser redirect through the provider's own OAuth or manifest flow. Once connected, these commands browse its repos and branches, connect one as an app's git source with `use-as-source`, or check and forget the connection.
- `disconnect` forgets the stored connection and revokes the token at GitLab. It does not uninstall the GitHub App, and Bitbucket and Gitea have no revocation API; the output says what applied.
- `git-providers` shows a `HEALTH` column. A connection that needs the operator (`needs_reauthorize`, `revoked`) prints the page to open to reconnect, since connecting is a browser redirect.
- `github-app register-url` prints the dashboard link that starts GitHub App registration, for your personal account or, with `--owner`, an organization. `--public` lets other accounts and organizations install the App. It makes no request; open the link in a signed-in browser.
- `github-app installations add` prints the URL to install the App on another account or org; it does not open a browser. `installations remove` is refused (409) while a git source still points at a repo under it.

## Templates

```
levelrail-cli templates list [--custom] [flags]
levelrail-cli templates get <id> [flags]
levelrail-cli templates deploy <id> [--name NAME] [flags]
levelrail-cli templates install <id> [--name NAME] [--env KEY=VALUE]... [--domain SERVICE=HOST]... [flags]
levelrail-cli templates delete <id> [flags]
```
`list` browses the curated service catalog, or `--custom` for your own saved templates. `get` shows one entry including its full compose.yaml. `deploy` deploys a template's compose.yaml as an app, and the app name defaults to the template id; pass `--name` to use another. A custom template works with every verb the same as a built-in one, except `delete`: only a custom template can be deleted (see `apps save-as-template`). See [Templates](/templates).

`install` is for manifest templates (the ones `get` shows with settings): it fills each `${KEY}` setting from `--env`, a generated secret or its default, puts the listed services behind `--domain` hosts, and deploys the result as a compose app that is not a trial. See [Compose apps](/compose-apps#templates).

## Compose

```
levelrail-cli compose deploy <app> -f compose.yaml [--profile-name P]... [flags]
levelrail-cli compose preview -f compose.yaml [--profile-name P]... [flags]
levelrail-cli compose status <app> [flags]
levelrail-cli compose revisions <app> [flags]
levelrail-cli compose logs <app> [--service S] [--since 1h] [--tail N] [-f] [flags]
levelrail-cli compose redeploy <app> [--service S] [flags]
levelrail-cli compose rollback <app> [--revision N] [flags]
levelrail-cli compose down <app> [--volumes] [flags]
```
- `deploy` deploys or updates a stack and records a numbered revision; `-f -` reads stdin. `--profile-name` enables services behind compose `profiles:`.
- `preview` asks the server to parse and check a file without saving anything, lists each service with its generated and required env vars, and exits with code 2 when the file would be rejected.
- `status` prints the stack phase and each service's ready state with the reconciler's reason.
- `logs` merges recent lines from every service, or hands one service to `apps logs` with `--service`, where `-f` follows it.
- `redeploy` recreates every service's containers, or one service's.
- `rollback` re-applies an earlier compose file (default: the one before the latest) as a new revision.
- `down` removes every service and its containers; named volumes and generated secrets are kept unless you pass `--volumes`.

See [Compose apps](/compose-apps).

## Static sites

```
levelrail-cli static-sites list [flags]
levelrail-cli static-sites routes show <app> [flags]
levelrail-cli static-sites routes test <app> <path> [--method M] [--request-query Q] [flags]
levelrail-cli static-sites routes lint <app> [--redirects FILE] [--headers FILE] [flags]
```
`list` shows every `build.type: static` app served directly through the embedded Caddy ingress, with no container involved. The `routes` commands show a site's redirect, rewrite and header rules, run one request through them, and check `_redirects` and `_headers` files without deploying. See [Static site routing rules](/static-routing).

## Response cache

```bash
levelrail-cli cache status [<app>] [flags]
levelrail-cli cache rules show|set|clear <app> [--domain D] [flags]
levelrail-cli cache purge <app> (--url PATH | --prefix PATH | --tag NAME | --all) [flags]
levelrail-cli cache history <app> [flags]
```

- `cache status` shows the hit rate and bandwidth saved for one app, or the instance table without a name. `cache rules set --preset static` adds the static assets rule; `--file` replaces the policy from JSON. Purges are recorded in `cache history`. See [CDN and caching](/cdn-and-caching).

## Images

```bash
levelrail-cli images list [--node N] [--keep N] [flags]
levelrail-cli images layers <image-id> [--node N] [flags]
levelrail-cli images gc [--node N] [--keep N] [--min-age-hours H] [--apply] [flags]
levelrail-cli images verify (<image-id> | --ref repo@sha256:...) [--node N] [flags]
levelrail-cli images policy [--ref repo@sha256:...] [flags]
```

- `images list` shows every image on a node with size, owners and why it is kept. `images gc` previews until `--apply` and never removes live images, rollback targets or recent deploys. `images verify` checks a cosign signature and exits non-zero when it fails. See [Images](/images).

## Related

<CardGroup cols={2}>
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
