# Comparison

Positioning, not a ranking. All of these projects are worth using. This
page exists to make the actual technical differences legible, sourced
from direct competitor research (cloned source, changelogs, and issue
trackers for each project, read at the level of detail this project's
own research already produced).

## At a glance

| Project | Node control | Orchestration | Observability | Ingress | License | Build engine |
| --- | --- | --- | --- | --- | --- | --- |
| Coolify (v4) | SSH plus CLI-shelled `docker`/`docker compose` via `instant_remote_process()`, every remote operation is a heredoc of shell commands over SSH | Docker Compose per app, cutover via Traefik label discovery (`rolling_update()`) | Optional bolted-on agent ("Sentinel"), opt-in, separately installed | Traefik, separate long-lived container, auto-discovers containers via Docker labels | Not stated in research | Nixpacks or Dockerfile (per-app config column) |
| Dokploy | SSH-tunneled Docker Engine API (via `dockerode`) for the core deploy call; CLI shelled over an `ssh2` exec channel for lifecycle ops, cleanup, and the build pipeline | Docker Swarm services: `service.update()`/`createService()`, Swarm's native `UpdateConfig`/`RollbackConfig`/`FailureAction: rollback` | Separate Go binary (`apps/monitoring`), polling ticker, container stats default every 60s | Traefik, run as a Swarm service on the shared overlay network | Not stated in research | Not stated in research |
| CapRover | Docker Swarm API via `dockerode`; `DockerApi.initSwarm()` runs even on a single node | Docker Swarm services, `AUTO` update order (stop-first if volumes mounted, start-first otherwise) | Optional sibling containers (NetData, GoAccess), not built into core | nginx, sibling Swarm service; config written to a `.fut` file, validated with `nginx -t`, then `SIGHUP`-reloaded | Not stated in research | Not stated in research |
| Dokku | SSH/git push triggers the local `dokku` bash entrypoint directly on the host; no persistent daemon, no separate node agent | Custom Bash scheduler (`scheduler-docker-local`) driving plain `docker` containers, no Swarm | Not stated in research | nginx by default (pluggable per-app: Caddy, HAProxy, Traefik, OpenResty), config rendered via `sigil` and reloaded with `nginx -s reload` | Not stated in research | Herokuish buildpacks (Heroku-style `app.json`/`Procfile`) or a Dockerfile |
| Kamal | One-shot SSH CLI session per command, no daemon, no agent, state re-derived live from `docker ps`/`docker inspect` every invocation | None: no scheduler or daemon. CLI runs `docker run` per host; `kamal-proxy` performs an in-memory atomic target swap after its own HTTP health probe | None built in; `kamal-proxy` exposes a bare Prometheus metrics port to scrape yourself, no query API | `kamal-proxy`, one standalone Go container per server, atomic in-process routing table update | MIT | Dockerfile-based build step (buildpacks requested via community PR, not shipped) |
| Levelrail | Reverse-dialed gRPC agent with mTLS; no CLI shelling anywhere, Docker Engine API wrapper only | Custom Go reconciler over the Docker Engine API, level-triggered, blue-green default strategy, rolling/recreate also supported | Node-local metrics (15s resolution) and full-text log store, federated query API, threshold alerting with multi-channel notifications, crashloop detection, Prometheus remote-read, all shipped | Embedded Caddy, driven in-process via its admin API; automatic internal-issuer TLS works today, a public ACME issuer and settings toggle are built but unverified against a live domain | Apache 2.0 (this project's own settled decision) | BuildKit-based Dockerfile builds with local cache, Railpack auto-detection (Node.js/Go), static-site serving |

## Levelrail vs Coolify

Coolify v4 drives every managed node over SSH, shelling `docker`/`docker compose` commands and parsing text output, and its per-deploy health check is off by default, so a broken deploy can be marked successful while the previously-working container is deleted underneath it. Levelrail never shells a CLI command against a node, and the reconciler only reports a cutover complete once the new container's readiness probe has actually passed. One honest caveat worth stating plainly: Coolify's own in-progress `v5.x` rewrite is already converging toward a similar shape, a Rust per-node agent (`coold`) talking gRPC to a hub, plus Caddy replacing Traefik as ingress, so the "SSHes and shells out `docker`" contrast is specifically a v4 claim, not a permanent one.

## Levelrail vs Dokploy

Dokploy is a thin control surface over Docker Swarm: its rolling update, rollback, and cross-node routing are all Swarm's own mechanisms, and a deployment is marked "done" the instant the Swarm API call returns, before there's any evidence the new task is actually healthy. Levelrail's reconciler owns cutover, rollback, and node placement directly rather than delegating to a cluster orchestrator, and a status condition is only written once the new container's readiness probe passes. Dokploy's own most-discussed GitHub issues cluster around networking and ingress fragility on Swarm's routing mesh plus a separately-configured Traefik container (Traefik breaking on restart, gateway timeouts on worker-node replicas). That is the exact failure class Levelrail's embedded, in-process Caddy plus its own WireGuard mesh are built to avoid.

## Levelrail vs CapRover

CapRover also standardizes on Docker Swarm, even for a single-node install, and has no per-app deployment health check at all: a deploy is considered successful once the image builds and the Swarm API call resolves, so a crashlooping app after deploy gets reported as a success and the operator finds out from a 502. There is also no rollback feature in CapRover's backend; the only way back is manually re-tagging and re-deploying an old image by hand. Levelrail gates cutover on the app spec's readiness and liveness probes, and ships rollback with pinned prior images as a first-class, always-available action rather than something an operator has to reconstruct.

## Levelrail vs Dokku

Dokku actually gets the deploy ordering right: new container up, health checks pass, proxy config regenerated and validated, then the old container is stopped. It also has genuinely zero idle cost since there is no persistent daemon at all; every command is a fresh SSH/git-triggered bash invocation. The tradeoff is that Dokku cannot do anything proactive between commands: no event-stream consumption, no background reconciliation, and its boot-time container recovery script is an admitted "temporary hack" the maintainers have carried since a years-old issue. Its default scheduler also has no rollback command whatsoever (rollback only exists on the newer, opt-in Kubernetes-backed scheduler). Levelrail's agent streams Docker events continuously rather than triggering only on command, and rollback with pinned images is available from Phase 1, not deferred to an alternate scheduler.

## Levelrail vs Kamal

Kamal is the deliberate outlier in this set: no daemon, no agent, no database, just a one-shot SSH CLI plus a standalone Go reverse-proxy container (`kamal-proxy`) that performs an in-memory atomic target swap once its own HTTP health probe passes. It is the lightest-weight design here and its two-stage health gate (container state, then an HTTP probe) is a pattern worth keeping, not simplifying away. But it is not a control plane: there is no persistent agent, no event-driven observed state, and no queryable metrics store (`kamal-proxy` exposes a bare Prometheus port to scrape yourself, nothing federated). Its single most-commented issue in the whole tracker (76 comments) is the SSH transport itself disconnecting mid-command, a failure category that doesn't exist without a reachable SSH session driving every deploy. Levelrail keeps Kamal's health-gate discipline but replaces the SSH transport with a persistent, reverse-dialed gRPC agent and adds the federated, node-local observability Kamal has no path to.

## Beyond the reconciler: operational surface

The sections above are about how each project talks to a node and cuts
over a deploy. That's the architectural core, but an operator running
this in production day to day also needs access control, alerting, and
managed data stores that don't fall over. This section is deliberately
scoped to describing what Levelrail itself has shipped, not a
line-by-line claim about what each competitor above does or doesn't
have in this area: that would need the same level of sourced research
the architecture table above got, and this project hasn't done that
research yet.

- **Access control.** An IAM-style policy engine
  (`internal/api/iam.go`) with AWS-IAM-shaped Allow/Deny statements,
  attachable to a user or an API token and scoped to a specific
  resource (`app:myapp`, `database:mydb`, or `*`), additive on top of a
  flat abilities list rather than replacing it. Three curated role
  presets (admin, operator, viewer) apply a full ability set in one
  action instead of hand-picking abilities. Every request, from the
  CLI, dashboard, MCP server, or raw API, runs through the same
  ability-check hook and lands in a queryable audit log with CSV
  export and configurable retention.
- **Feature flags.** A boolean plus an optional gradual rollout
  percentage, scoped to an app, read live at runtime via
  `GET /api/v1/flags/evaluate/{key}` and a read-scoped API token. No
  redeploy or restart, since the value is never baked into a
  container image.
- **Alerting.** Eight rule kinds (threshold, crashloop, certificate
  expiry, OS patch status, scheduled-task failure, node disk space,
  node resource usage, and domain health) and thirteen notification
  channel kinds (webhook, Slack, Discord, email, Telegram, Pushover,
  PagerDuty, Microsoft Teams, Resend, ntfy, Gotify, Mattermost, Lark),
  each independently queryable for delivery history.
- **Managed databases.** Eight engines (Postgres, Redis, MySQL,
  MongoDB, MariaDB, KeyDB, Dragonfly, ClickHouse) through one dynamic
  engine registry, not eight separate implementations: scheduled
  backups with retention, restore, restore-into-a-new-resource, and
  automatic post-backup verification (re-download, re-hash, compare
  against what was recorded at backup time) apply generically across
  all eight.

None of this changes the answer to "why build a new one instead of
using an existing platform" above, that answer is still the
architecture. It's here because a reconciler that never SSHes into a
box is not, by itself, a reason to trust a tool with production
secrets and access control, and this is the evidence for the second
half of that trust.

## What Levelrail doesn't do (yet)

Grounded in this project's own current feature and build status, not the original phase plan, since the actual build is further along in places than that plan suggests (multi-node and the WireGuard mesh are shipped, not "not started"). See `docs/roadmap.md` for the full current status; the real gaps as of today:

- **Real public ACME certificates, verified.** TLS today defaults to an internal, self-signed issuer. A real Caddy ACME issuer, a settings toggle, and form validation are built and wired end to end, but issuance against a live domain has only been unit-tested at the config level, not spot-checked against a real domain.

## Feature matrix

The "Beyond the reconciler" section above named its own gap: everything
in it described Levelrail, not a line-by-line claim about the five
competitors. This table closes that gap, using the same method as the
architecture table at the top of this page: every competitor claim comes
from reading the cloned source directly, a Laravel/Livewire app for
Coolify, a TypeScript monorepo for Dokploy, a Node.js/TypeScript app for
CapRover, a Bash plugin tree for Dokku, and a Ruby gem for Kamal, at
`docs-local/competitor-clones/`, not from general knowledge of these
products. Where a clone didn't contain enough evidence to answer
confidently, the cell says "not stated in research" rather than
guessing. Levelrail's own column is sourced from `docs/roadmap.md`
(current as of 2026-09-01), cross-checked against the file paths cited
on `main`. Full per-competitor citation detail beyond what fits here
lives in `docs-local/research/prior-art-feature-matrix.md`.

| Feature | Levelrail | Coolify | Dokploy | CapRover | Dokku | Kamal |
| --- | --- | --- | --- | --- | --- | --- |
| Multi-user accounts / teams, invite model | Yes, real accounts (email or OAuth). Admin-created only: `handleRegister` bootstraps just the first user, every later one via `POST /api/v1/auth/users` or OAuth login, no invite system by design (`internal/api/auth.go:569-578`) | Yes, team-scoped, but invite is admin/owner-initiated, not open signup: `InviteLink::generateInviteLink()` pre-creates the invited `User` (`app/Livewire/Team/InviteLink.php:77-87`) | Yes, and genuinely self-service among permitted members: any member holding `member.create` can invite (`organization.ts:300` `inviteMember`) | No. Single-tenant by design: `UserManagerProvider.get()` throws for any namespace but root (`src/user/UserManagerProvider.ts:14-19`) | No accounts, only admin-added SSH keys (`ssh-keys:add`); no self-service path | Not applicable, no user/team concept at all (local CLI over the operator's own SSH access) |
| RBAC granularity | Fine-grained, resource-scoped: IAM-style Allow/Deny policies attachable to a user or token, scoped to `app:name`/`database:name`/`*` (`internal/api/iam.go`, `internal/store/iam_policy.go`), additive on top of 3 role presets | Coarse: one team-wide role column (owner/admin/member), no per-app scoping (`Team::members()` pivot, `app/Models/Team.php:258-260`; policies check `isAdminOfTeam`) | Fine-grained on paper, roughly 30 resources with distinct actions (`access-control.ts:14-52`), but custom per-resource roles require a paid license; the free tier gets 3 fixed roles | None, moot with one account: `UserJwt` carries only `namespace`/`tokenVersion`, no role field (`src/models/UserJwt.ts`) | None in core: binary admin-vs-not by SSH key naming convention; per-app ACL is a `user-auth` trigger stub only community plugins implement | Not applicable, no identity system to scope |
| One-click template catalog (count) | 15 hand-authored templates (`internal/catalog/catalog.go`), served over `GET /api/v1/service-templates` | 371 files in `templates/compose/` | Fetched at runtime from `templates.dokploy.com` (`packages/server/src/templates/github.ts`), not bundled; count not verifiable from this clone | Mechanism exists (`OneClickAppDeployManager.ts`) but the manifest lives in a separate, uncloned repo; count not stated in research | None bundled; roughly 80+ separate official plugin repos, each installed individually, no in-product browse-and-install catalog | None; `kamal init` only scaffolds one app's own `deploy.yml`, not a service catalog |
| Docker Compose as a deploy target | Yes (`internal/compose`, multi-service `DeploySpec` fan-out) | Yes, first-class `build_pack` option (`app/Models/Application.php:47`) | Yes, first-class, dedicated `compose` table/router (`packages/server/src/db/schema/compose.ts:33-58`) | No for user apps: `ICaptainDefinition` accepts a Dockerfile/image/template, one of four, never compose (`src/models/ICaptainDefinition.ts`); compose syntax is reused only inside the one-click template DSL | No; strictly single-container-per-process-type via Dockerfile/Procfile across every `builder-*` plugin | No; own `deploy.yml` schema only, no compose parser found |
| Managed database engines | 8: Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB, Dragonfly, ClickHouse, one dynamic engine registry | 8: Postgres, MySQL, MariaDB, MongoDB, Redis, KeyDB, Dragonfly, ClickHouse (`app/Models/Standalone*.php`), essentially the same list as Levelrail | 5: Postgres, MySQL, MariaDB, MongoDB, Redis | None first-class; databases run as ordinary one-click apps with no dedicated backup/credential/resource-limit wiring | None in core; separate official plugins per engine (dokku-postgres, dokku-mysql, dokku-mongo, and similar), opt-in installs | None; "accessories" are generic sidecar containers the operator configures by hand, no managed backups or credentials |
| Scheduled backups with automated restore verification | Yes: scheduled/cron backups across all 8 engines plus app volumes, with automatic post-backup verification (re-download, re-hash, compare checksum/size/format against what was recorded) after every run, and restore/restore-as-new | Scheduled, but not verified: only a non-zero-byte-size check (`DatabaseBackupJob::calculate_size()`), no checksum or test-restore; no restore job found anywhere in the codebase | Scheduled (BullMQ cron workers), but no automated verification step; restore is a separate, manually-triggered action | On-demand only, no scheduling in core (`BackupManager.ts` has no cron); a real restore path exists, but zero automated verification | None in core; the backup plugin was deprecated in 0.4.x, current guidance is a manual `tar` of the data directory with manual integrity checking | None; no backup feature exists anywhere in the codebase |
| Preview environments per pull request | Yes, opt-in per app. GitHub path is fully end-to-end tested; GitLab/Bitbucket payloads are parsed but not yet e2e-verified. Scheduled TTL sweep, dashboard and CLI wiring | Yes (`app/Models/ApplicationPreview.php`, webhook handling across GitHub/GitLab/Gitea) | Yes, full CRUD router plus GitHub webhook handling for opened/synchronize/reopened/closed (`preview-deployment.ts`) | No; zero matches for `preview`/`pull_request` in source | No in core; the only "preview" hit is an unrelated k3s-scheduler manifest-diff command, not a per-PR ephemeral environment. The documented review-apps pattern is an external CI recipe | No; zero matches for `preview` in source |
| Feature flags / runtime config without redeploy | Yes: boolean plus gradual rollout percentage, scoped to an app, read live via `GET /api/v1/flags/evaluate/{key}`, never baked into the container image | No dedicated flag system found, only ordinary DB-backed settings columns | No; no feature-flag concept found | No operator-facing flag system; the only flag found is an internal vendor kill-switch gating the paid Pro tier | Partial in name only: `config:set --no-restart` skips the container restart, but the running container then simply never sees the new value until next recreate, no live-read path | No; every env change requires a fresh `kamal deploy` since containers read env once at boot |
| Alerting rule kinds and notification channel kinds | 8 rule kinds (threshold, crashloop, certificate expiry, OS patch status, scheduled-task failure, node disk space, node resource usage, domain health) and 8 channel kinds (webhook, Slack, Discord, email, Telegram, Pushover, PagerDuty, Microsoft Teams) | Mostly fixed lifecycle events (deployment failed/success, restart-limit-reached, backup failed, server unreachable) plus one real threshold rule (disk usage percent); 6 channels: Discord, Slack, Telegram, Pushover, email, generic webhook | Fixed lifecycle booleans (deploy, build error, backups, restart, cleanup) plus one threshold rule (server CPU/memory, via a separate Go monitoring binary); 12 channels: Slack, Telegram, Discord, email, Resend, Gotify, Ntfy, Mattermost, Pushover, custom webhook, Lark, Teams | Split across two disconnected systems: node-resource alerts fully delegated to a sibling NetData container (no rules of CapRover's own), plus a subscription-gated Pro event system (login, build success/fail) with only email/webhook channels | None in core; no alert/notification plugin found | None built in; only a lifecycle-hook mechanism (pre-deploy, post-deploy, and similar) a user can wire to their own notification script by hand |
| CLI tool maturity | `--output json\|table\|text`, `--query` (JMESPath), shell completion for bash/zsh/fish, and an RFC-8628-shaped device-login flow (`auth login --device`) | No standalone CLI exists at all | No standalone CLI exists at all | A separate `caprover` CLI package exists but its source isn't in this clone; not stated in research at the code level | The CLI is the whole product: 22+ subcommands support `--format json`, bash completion ships (`contrib/bash-completion`); no device-login flow, access is SSH-key based by design | Mature Thor-based CLI (`kamal config`, `kamal docs`) but no `--json`/structured output and no shell-completion script found; SSH-key based, no device-login |
| MCP server / AI-agent-facing API layer | Yes, `cmd/levelrail-mcp`, 33 tools, same bearer-token/ability model as the REST API and CLI | Yes, and larger: `app/Mcp/Servers/CoolifyServer.php`, roughly 45 tools, gated per-team by a settings flag | No; zero matches for `mcp` in the repo | No | No, and no REST API at all by design | No; no HTTP API of any kind |
| Audit logging with per-request attribution | Yes: every request through the single ability-check hook lands in a queryable audit log, attributed to a client kind (cli/dashboard/mcp/api) via the User-Agent header, CSV export, configurable retention with a periodic sweeper | No general audit trail; only narrower activity/deployment command-output logs (`spatie/laravel-activitylog`), no actor-attribution for arbitrary mutations | Yes, but narrower and paid: an `auditLog` table records user/action/resource (`packages/server/src/db/schema/audit-log.ts`), no IP/user-agent/client-kind, and the whole feature is gated behind an enterprise license | No; only anonymous usage telemetry sent to CapRover's own servers, explicitly excluding sensitive events | Partial, opt-in, unstructured: an events log attributes each plugin-trigger call to the SSH key's name/fingerprint, but it's off by default and unqueryable (`dokku events:on`) | Partial but decentralized: a per-host, per-service log file via `auditor.rb`, attributed to a git-email or shell username, not a central request-attributed log |
| API token scoping | Fine-grained abilities (`read`, `read:sensitive`, `write`, `write:sensitive`, `deploy`, `root`) plus resource-scoped IAM policies attachable to a token | Named ability strings (`read`, `read:sensitive`, `write`, `write:sensitive`, `deploy`) via Sanctum, no resource scoping beyond that | All-or-nothing in practice: an `apikey.permissions` column exists in the schema but token-creation code never sets it, so a key just inherits its user's full role | No scoping; purpose-tagged JWTs (cookie vs webhook vs download) but no capability distinction within the admin session | Not applicable, no REST API or token concept | Not applicable, no REST API or token concept; SSH-key access only |

Three rows in this table are genuine, current gaps for Levelrail, not
architectural differences it can shrug off: the template catalog (15
curated entries against Coolify's 371, an intentional curation-over-count
bet per ADR 015, but a real breadth gap all the same), the MCP tool count
(33 against Coolify's roughly 45), and notification channel breadth (8
against Dokploy's 12, missing Resend, Gotify, Ntfy, Mattermost, and Lark
specifically). Dokploy's fine-grained RBAC, self-service invites, and
per-request audit log are also real, working functionality, worth naming
plainly rather than waving off, though all three sit behind a paid
enterprise license upstream rather than Dokploy's free tier, which
matters for an apples-to-apples read.

Everywhere else in this table, Levelrail matches or leads, and the
backup-verification row is the sharpest example: none of the other five
projects researched here run any automated check on a backup after
taking it. Coolify checks only that the dump file is non-empty, Dokploy
and CapRover do no check at all, and Dokku and Kamal have no built-in
backup feature in the first place.
