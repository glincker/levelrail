# Deploying and managing apps

An app is one `store.DesiredService` row: an image, a port, and everything
the application controller needs to converge a running container to it.
Package: `internal/api/apps.go`, `apps_multi.go`, `apps_compose.go`,
`deploys.go`, `promote.go`, `exec.go`, `resources_live_apply.go`,
`scheduled_tasks.go`; CLI: `cmd/levelrail-cli/apps*.go`; dashboard:
`web/src/routes/apps/$name/*.tsx`.

This doc covers an app's own lifecycle: creating it, deploying to it,
rolling it back, promoting it between environments, starting/stopping/
restarting/deleting it, its health checks, its resource limits, one-off
exec, and scheduled cron tasks inside its container. Domains and TLS,
managed databases, and CI/git-provider integrations each have their own
doc; this one links out rather than duplicating them.

## Why "deploy" has no separate "rollback" endpoint

There is exactly one mutation that changes what image an app runs:
`POST /api/v1/apps/{name}/deploys`. It points `desired.Image` at whatever
tag you send and returns immediately; the application controller's next
reconcile pass is what actually creates the new container. A rollback is
just that same call with an older, already-built tag instead of a newer
one, converging exactly the same way. There is no image-history endpoint
that resolves "the previous tag" for you, in the API or the CLI: you have
to already know which tag you're rolling back to, either from
`GET /api/v1/apps/{name}/deploy-attempts` (the dashboard's Deploy history
list keeps this visible with a one-click "Rollback to this build" per
row) or from your own build records.

`apps rollback` exists in the CLI anyway, and the dashboard's rollback
button is a real button, because a caller who wants to type "rollback"
and see rollback-framed output shouldn't have to remember it's spelled
"deploy". Both are thin wrappers over the one mechanism, not a second
code path that could drift from it.

## How it actually works

`SaveDesiredService` is a full replace, not a patch: `PUT /api/v1/apps/{name}`
overwrites every field with whatever the request body carries, the same
as an app.yaml apply. Fields the general update endpoint doesn't own
(`node_id`, `project_id`, `environment_id`, `storage_target_id`,
`suspended`, `database_attachment`, `log_drain`) are response-only there
and have their own dedicated endpoint, so an ordinary edit can never
silently move an app between nodes or projects.

Every state-changing action funnels through a small set of primitives:

- **Redeploy** (`setDesiredImage`): overwrite `Image`, clear `EnvDirty`,
  save. Deploy, rollback, and promote all call this.
- **Suspend/resume** (`UpdateServiceSuspended`): flips `Suspended`. The
  reconciler, not the handler, actually stops or restarts the container
  on its next pass.
- **Restart** (`RestartService`): mints a fresh `restart_nonce`, folded
  into the container name hash, which makes "same image, new nonce" look
  exactly like an image change to the existing blue-green/recreate
  cutover logic. This is the only way to force a container recreation
  with zero image change.
- **Delete** (`DeleteDesiredService`): removes the desired-state row, then
  the handler itself (not the reconciler) tears down the app's current
  containers in a background goroutine, and deletes the app's `store.App`
  row too if this was its last member service.

`EnvDirty` is the one piece of drift tracking: saving new env vars without
also changing the image sets it, and it stays set until a real redeploy or
restart lands. The dashboard surfaces this as an amber "Environment
changes pending restart" banner with a one-click restart action, at the
top of every app's Overview page.

## The four ways to create an app

All four end up as one or more `store.DesiredService` rows under one
`store.App`. The dashboard's "New" wizard (`CreateResourceWizard.tsx`)
offers three of them as step-1 cards plus a curated template catalog
(out of scope here, see the templates doc); the fourth (a multi-service
`app.yaml`) is reached from an existing app's Services tab, not the
initial wizard, because it fans out under an app that already exists.

1. **Existing Docker image.** `POST /api/v1/apps` with `image`, `port`,
   and no build step at all. Dashboard: the "Docker image" wizard card
   (`CreateAppFields.tsx`). CLI: `apps create --name NAME --image IMAGE
   --port PORT`.

2. **Git-repo build.** `POST /api/v1/apps` with a `:pending` placeholder
   image, immediately followed by `POST /api/v1/apps/{name}/builds` (a
   real Dockerfile or Railpack build via BuildKit) which overwrites that
   placeholder with the real built tag once it succeeds. Dashboard: the
   "Deploy from git" wizard card (`CreateAppFromGitFields.tsx`). CLI:
   `apps create --name NAME --port PORT --repo URL --image-repo REPO`
   (auto-detects `origin` and the current branch when run inside a git
   checkout), or `apps create --file app.yaml --service KEY` for a
   single service out of a spec file.

3. **Docker Compose.** `POST /api/v1/apps/{name}/compose` with a raw
   `compose.yaml` body: each compose service becomes its own
   `DesiredService` under one app, in one synchronous call, with its own
   `deploy_attempts` row (`store.DeployAttemptSourceCompose`). Compose
   "magic vars" (`SERVICE_<KIND>_<KEY>`) that need a generated secret are
   resolved and persisted automatically; a compose file that bind-mounts
   a host directory additionally requires the `root` ability on top of
   `deploy`. Anything compose declares but Levelrail can't faithfully
   translate (a health check with no readiness-probe equivalent, for
   example) comes back as a `notices` entry rather than failing or being
   silently dropped. Dashboard: the "Docker Compose" wizard card
   (`CreateComposeFields.tsx`). CLI: `apps deploy-compose <name> --file compose.yaml`.

4. **`app.yaml` deploy-spec (multi-service).** `POST
   /api/v1/apps/{name}/deploy-spec` takes a git repo/ref plus an
   `app.yaml`-shaped `services:` map and builds+deploys every service in
   it, in deterministic key order, under one app. Unlike Compose, this is
   synchronous per service and does not (yet) write a `deploy_attempts`
   row per service, only the fan-out response itself records what
   happened; one service's build failing does not block the others, and
   the response is `207 Multi-Status` when at least one did. Dashboard:
   an existing app's Services tab (`DeploySpecForm.tsx`), alongside the
   sibling-services list. CLI: `apps deploy-spec <name> --file app.yaml --repo-url URL --ref REF`.

All four accept inline secret values (`--secret KEY=VALUE` on the CLI,
`secrets` on the wire) stored via envelope encryption as part of the same
create/deploy call, so a `{ secret: true }` env var declared in the spec
doesn't need a separate follow-up call to set its value.

## External secrets: HashiCorp Vault

`{ secret: true }` above stores the value itself, encrypted at rest,
inside Levelrail. As an alternative, an env var can instead resolve its
value live from an external HashiCorp Vault instance, never storing the
value at all:

```yaml
env:
  API_KEY: { vault: { path: myapp/config, key: api_key } }
```

`path` is a secret's path in Vault's KV v2 engine; `key` is the field
name inside that secret's data. See [`EnvVar`/`VaultRef` in the app.yaml
reference](app-spec-reference.md#envvar-an-entry-under-env) for the full
field table. `vault` is mutually exclusive with both `from` and `secret`
on the same env var.

**Setup.** Configure the connection once, instance-wide, under
Settings → Vault (dashboard), `levelrail-cli vault set`, or `PUT
/api/v1/settings/vault`: a Vault address, an auth method (`token` or
`approle`), and a credential (a Vault token, or an AppRole role ID plus
secret ID). The credential is stored the same way every other
platform-wide credential is (envelope-encrypted, write-only over the
API), never logged, never written to disk in plaintext.

**Resolution.** A vault-backed env var's value is read fresh from Vault
by the reconciler immediately before the container is created, the same
"resolved at container-create time, never persisted" trust model
`{ secret: true }` already follows. If Vault is unreachable, not
configured, disabled, or the secret/field doesn't exist, the deploy
fails loudly with a clear error rather than starting the container with
the variable silently empty or omitted.

**Declaring one on an existing app.** Beyond `app.yaml`'s own `vault:`
syntax, an app created directly (not from `app.yaml`) can declare or
remove a Vault-sourced env var at any time: the dashboard's app
Environment tab, `levelrail-cli apps vault-env set|clear <name> <key>`,
or `PUT`/`DELETE /api/v1/apps/{name}/vault-env/{key}`. This never
touches any other field on the app, unlike the general update endpoint.

## Lifecycle actions

| Action | What it does | Not to confuse with |
| --- | --- | --- |
| Deploy | Points `Image` at a new tag, saves, returns immediately | Restart (no image change) |
| Rollback | Same call as deploy, given an older tag | A dedicated undo mechanism (doesn't exist) |
| Promote | Points a sibling app in another environment at this app's current image | Deploy (different target app) |
| Restart | Recreates the running container, same image | Deploy/rollback (both only act when the image actually changes) |
| Stop | Sets `Suspended`; reconciler stops the container next pass | Delete (desired state still exists) |
| Start | Clears `Suspended` | Create (app already exists) |
| Delete | Removes the desired-state row; containers torn down in the background | Stop (state is gone, not just paused) |

**Promote** (`POST /api/v1/apps/{name}/promote`) is scoped to one project:
it finds a sibling app tagged with the destination `--to` environment ID
in the *same project* as the source app (there's no declared 1:1 "this
app in another environment" relationship, since apps are independently
named), auto-discovering the sole candidate or requiring `--target` when
there's more than one. `GET .../promote/preview` shows what would change
before you commit to it, and it's honest that only the image tag is ever
compared: env vars, ports, domains, and resource limits are the target
app's own settings and are never touched by a promotion.

Both deploy and promote respect **protected environments**: if the app
(or promotion target) is tagged with one, the request needs `confirm:
true` in the body or it fails with a 409; the CLI falls back to an
interactive "yes" prompt on stdin when `--confirm` isn't given.

The dashboard's per-app header (`routes/apps/$name.tsx`) puts stop/start,
restart, promote, clone, and delete as one-click buttons next to the app
name and status badge; a pinned "Trigger a deploy" form sits above the
Overview and Deploys sections for the deploy/rollback path.

## Health checks

`store.ServiceHealth` holds up to two `ServiceProbe`s, `Readiness` and
`Liveness`, each a path plus optional interval/timeout/failure-count.
Both are wire-encoded as plain `time.Duration` JSON, i.e. **nanoseconds**,
not seconds or a duration string; the dashboard's Health tab
(`HealthCheckEditor.tsx`) converts to/from whole seconds for the input
fields. A probe is either absent (`null`) or fully specified with at
least a path; there's no partial "path only, defaults for the rest" on
the wire, though the dashboard defaults interval/timeout/failures to
sensible values when you leave them blank.

Set health checks through the same `PUT /api/v1/apps/{name}` full-replace
call every other field goes through (there's no dedicated health
endpoint); the CLI's `apps create --file app.yaml` path picks these up
straight from the spec's own `health:` block.

A deploy waiting on a readiness probe doesn't just retry HTTP requests
against a container that's already gone: it also watches the
container's own live state, so one that gets OOM-killed or otherwise
exits mid-wait fails immediately with a specific reason
(`OOMKilledDuringReadiness` or `ExitedDuringReadiness` in the deploy's
reconcile condition, visible in the dashboard's deploy history and
`apps deploys` in the CLI) instead of a generic `ReadinessFailed` only
after the full readiness budget (60s by default) has been spent
retrying a dead address.

## Resource limits and auto-recommendation

`store.ServiceResources` covers four dimensions: `memory_bytes`,
`nano_cpus` (CPU, as billionths of a core), `swap_memory_bytes` (Docker's
`MemorySwap`, memory+swap combined, must be >= the memory limit when
both are set), and `cpuset_cpus` (Docker's `cpuset-cpus` pin format, e.g.
`"0-3"` or `"0,2"`). All four are optional independently; leaving one off
runs that dimension unbounded. Set through the same full-replace `PUT`,
dashboard: the Resources tab's `ResourceLimitsEditor.tsx`.

Saving new limits tries to apply them **live**, without waiting for a
recreate: `applyResourcesLiveToReplicas` pushes them onto every currently
running replica via the Engine API's `ContainerUpdate` immediately after
the save succeeds. The response's `resources_applied_live` field tells
you whether that live push actually landed on at least one replica; if
it didn't (no container running yet), the values are still saved
correctly and take effect at the next create, which is what the
dashboard's fallback "restart required" toast is telling you.

**Auto-recommendation** (`GET /api/v1/apps/{name}/resource-recommendation`)
is a read-only, deterministic suggestion engine (`internal/rightsizing`),
not an LLM and not applied automatically: it looks at the app's own
memory/CPU usage samples over a lookback window (default 7 days), computes
p95/p99 per dimension against the current limit, and also checks recent
log entries for an OOM-kill signature. Each dimension's result carries a
sample count, a `data_sufficient`/`confidence` pair, and a suggested
limit with a plain-English reason; nothing here writes to the app, ever,
the operator decides whether to act on it. Dashboard: the Resources tab's
`ResourceRecommendationCard.tsx`, above the limits editor. CLI: `apps resource-recommendation <name>`.

## One-off exec and the interactive terminal

`POST /api/v1/apps/{name}/exec` is deliberately narrow: send a `command`
(plus optional `args`, never shell-interpreted, exactly `argv`), wait for
it to finish, get `stdout`/`stderr`/`exit_code` back in one response. A
nonzero exit code is still a `200`: the exec mechanism worked, the
command it ran chose to fail. Output is capped at 1 MiB (`truncated:
true` past that) and the whole call is bounded by a 30-second server-side
ceiling that a caller may only shorten (`timeout_seconds`), never extend.
Gated at the `root` ability, not `deploy`: secrets are injected as
plaintext env vars at container-create time, and `env` inside a shell
would read them straight back out, so exec sits at the same trust tier
as node management, not the deploy tier.

For anything beyond a single scripted command, `GET
/api/v1/apps/{name}/terminal` upgrades to a real interactive terminal
over WebSocket: a PTY, resize events, a shell kept alive, arrow keys and
Ctrl-C all working, the same `root` gate. Dashboard: the Exec tab shows
both, the interactive terminal (`AppTerminal.tsx`) above the one-shot
runner (`ExecPanel.tsx`). CLI: `apps exec <name> -- <command> [args...]`
for the one-shot path (exits with the *remote command's* real exit code,
not a generic CLI code), or `apps exec <name> --interactive [-- <shell>]`
for the terminal.

## Scheduled tasks

A scheduled task is an arbitrary `argv` command, no shell involved, run
inside an app's *currently running* container on a standard 5-field cron
expression (`internal/cronexpr`), the cron-inside-a-container feature.
Each task tracks its own `last_run_at`/`last_run_status`/`last_run_output`
and a `consecutive_failures` counter that a `scheduled_task_failure`
alert rule can watch. "Run now" (`POST .../scheduled-tasks/{id}/run`)
executes the identical code path a real cron tick uses
(`ScheduledTaskRunner.Run`), so there is exactly one implementation of
"exec this task's command and record the outcome," dispatched from a
detached background goroutine and answered with `202 Accepted`
immediately, not once the command finishes. Updates are a full replace,
like everything else in this doc: `command`, `schedule`, and
`concurrency_policy` must be resupplied on every `PUT`, not just the
field you're changing. Dashboard: the Scheduled tasks tab
(`ScheduledTasksPanel.tsx`). CLI: `apps
scheduled-tasks create|list|get|update|delete|run`.

`concurrency_policy` (`allow` | `forbid` | `replace`, default `allow`)
governs what happens when a task's next due run finds a previous
invocation of itself still executing, tracked in-memory per task ID
(`internal/scheduledtask.Runner`), the same overlap guard Dokku calls
`concurrency_policy`. `allow` starts the new run unconditionally, today's
behavior if you never set this field. `forbid` skips the new run and
records a distinct `skipped_concurrency` history status instead of
silently doing nothing. `replace` cancels the in-flight run (recorded as
`replaced`) before starting the new one; cancellation is the same
best-effort signal Runner's own timeout handling already relies on
(closing the exec stream), not a hard kill, since the Docker Engine API
has no "kill this exec" call.

## API reference

| Method | Path | Ability |
| --- | --- | --- |
| `POST` | `/api/v1/apps` | `write` |
| `GET` | `/api/v1/apps` | `read` |
| `GET` | `/api/v1/apps/{name}` | `read` |
| `PUT` | `/api/v1/apps/{name}` | `write` |
| `DELETE` | `/api/v1/apps/{name}` | `write` |
| `POST` | `/api/v1/apps/{name}/compose` | `deploy` (+ `root` if the compose file bind-mounts a host directory) |
| `POST` | `/api/v1/apps/{name}/deploy-spec` | `deploy` (+ `root` if any service bind-mounts a host directory) |
| `POST` | `/api/v1/apps/{name}/builds` | `deploy` |
| `POST` | `/api/v1/apps/{name}/deploys` | `deploy` |
| `GET` | `/api/v1/apps/{name}/deploys` | `read` |
| `GET` | `/api/v1/apps/{name}/deploy-attempts` | `read` |
| `GET` | `/api/v1/apps/{name}/deploys/compare?from=ID[&to=ID]` | `read` |
| `GET` | `/api/v1/apps/{name}/deploys/{deployId}/logs` | `read` |
| `GET` | `/api/v1/apps/{name}/promote/preview?to=ENV_ID[&target=NAME]` | `read` |
| `POST` | `/api/v1/apps/{name}/promote` | `deploy` |
| `POST` | `/api/v1/apps/{name}/restart` | `deploy` |
| `POST` | `/api/v1/apps/{name}/stop` | `deploy` |
| `POST` | `/api/v1/apps/{name}/start` | `deploy` |
| `PUT` | `/api/v1/apps/{name}/node` | `root` |
| `POST` | `/api/v1/apps/{name}/exec` | `root` |
| `GET` | `/api/v1/apps/{name}/terminal` (WebSocket upgrade) | `root` |
| `GET` | `/api/v1/apps/{name}/resource-recommendation` | `read` |
| `POST` | `/api/v1/apps/{name}/scheduled-tasks` | `write` |
| `GET` | `/api/v1/apps/{name}/scheduled-tasks` | `read` |
| `GET` | `/api/v1/apps/{name}/scheduled-tasks/{id}` | `read` |
| `PUT` | `/api/v1/apps/{name}/scheduled-tasks/{id}` | `write` |
| `DELETE` | `/api/v1/apps/{name}/scheduled-tasks/{id}` | `write` |
| `POST` | `/api/v1/apps/{name}/scheduled-tasks/{id}/run` | `deploy` |

## CLI

```bash
levelrail-cli apps create [flags]              # existing image, git build, --file, or --interactive
levelrail-cli apps list [flags]
levelrail-cli apps get <name> [flags]
levelrail-cli apps deploy <name> --image IMAGE [--confirm] [flags]
levelrail-cli apps rollback <name> --image IMAGE [--confirm] [flags]
levelrail-cli apps promote <name> --to ENVIRONMENT_ID [--target NAME] [--preview] [--confirm] [flags]
levelrail-cli apps restart <name> [flags]
levelrail-cli apps stop <name> [flags]
levelrail-cli apps start <name> [flags]
levelrail-cli apps delete <name> [flags]
levelrail-cli apps deploy-compose <name> --file compose.yaml [flags]
levelrail-cli apps deploy-spec <name> --file app.yaml --repo-url <url> --ref <ref> [--image-repo-base BASE] [--secret KEY=VALUE] [flags]
levelrail-cli apps resource-recommendation <name> [flags]
levelrail-cli apps exec <name> -- <command> [args...] [--timeout N] [flags]
levelrail-cli apps exec <name> --interactive [-- <shell> [args...]]
levelrail-cli apps scheduled-tasks create <app> --schedule CRON [--disabled] [--concurrency-policy allow|forbid|replace] -- <command> [args...]
levelrail-cli apps scheduled-tasks list <app> [flags]
levelrail-cli apps scheduled-tasks get <app> <id> [flags]
levelrail-cli apps scheduled-tasks update <app> <id> --schedule CRON [--disabled] [--concurrency-policy allow|forbid|replace] -- <command> [args...]
levelrail-cli apps scheduled-tasks delete <app> <id> [flags]
levelrail-cli apps scheduled-tasks run <app> <id> [flags]
```

Run `levelrail-cli apps <subcommand> -h` for a subcommand's own flags.
Domains/TLS live under `apps domains` (separate doc); databases under
`apps <db-verb>`/`databases` (separate doc); git-provider connections
under `apps git-source` (separate doc).

## Not built yet (deliberate follow-ups)

- **No image-history lookup.** Neither the API nor the CLI can tell you
  "the previous tag" for a rollback; you supply the exact tag yourself,
  from the deploy-attempts list or your own records.
- **No per-service deploy history for `deploy-spec`.** Unlike Compose,
  a multi-service `app.yaml` fan-out doesn't write a `deploy_attempts`
  row per service today, only the one synchronous response tells you
  what happened. A real per-service attempt log is a known, deliberately
  deferred store-schema change.
- **`GET /apps/{name}/deploys` is current reconcile status, not a deploy
  log.** It stores only the latest condition per controller/type pair,
  no history table; `GET .../deploy-attempts` is the real append-only
  history endpoint, a separate route on purpose.
- **Exec has no real interactive follow-up beyond the terminal route.**
  The one-shot `/exec` endpoint is intentionally not a shell: no PTY, no
  resize, no kept-alive session; that's what `/terminal` is for. A
  genuinely hung remote command (blocked on its own container-side read)
  isn't guaranteed to stop the instant the client's timeout fires,
  only the HTTP handler's own goroutine is guaranteed to return on time.
- **Deploy-spec and Compose are both synchronous, blocking calls.**
  Neither streams build progress over SSE the way the single-service
  build trigger does; a large multi-service spec is a genuinely slow
  HTTP request, not a fire-and-forget one.
- **Scheduled task history is last-run-only.** There's no run log beyond
  the current `last_run_at`/`last_run_status`/`last_run_output` and a
  consecutive-failure counter, no historical list of every past run.
- **`concurrency_policy: replace`'s cancellation is best effort, not a
  guaranteed kill.** It closes the previous run's exec stream, the same
  lever the run's own timeout already uses; a command that ignores its
  stream closing (blocked on a container-side read) keeps running inside
  the container even though Runner itself has moved on and recorded
  `replaced`. A real kill would need a mechanism the Docker Engine API
  doesn't expose for `docker exec` sessions.
