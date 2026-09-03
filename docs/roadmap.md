# Roadmap

Status as of 2026-09-01 (refreshed against current `main`), not the
aspirational plan. See CLAUDE.md in the
repo for the full phase-by-phase design doc if you want the long version.
The build has moved further and less linearly than that phase plan
implies: parts of Phase 3 (multi-node, the WireGuard mesh) are shipped
while some Phase 1 items (real public ACME, full multi-service support)
are still open. This page describes what's actually true today.

## Done

**Core deploy path**

- App spec parsing and validation (`app.yaml`/`deploy.yaml`) with an
  embedded JSON Schema.
- Docker Engine API wrapper. No shelling out to the `docker` CLI
  anywhere.
- Application reconciler: desired state to running containers, with
  readiness and liveness probes.
- BuildKit-based Dockerfile builds with local cache and live build-log
  streaming over SSE.
- Railpack auto-detection for Node.js and Go.
- Static-site builds (`build.type: static`), with a frontend surface to
  configure them (a static tab in the git build-source picker) and a
  dashboard card listing existing static sites.
- Docker Compose as a deploy target: `internal/compose` parses a
  supported subset of `compose.yaml`, including per-service `build:`
  directives, and expands it into per-service builds/deploys via the
  same multi-service fan-out `DeploySpec` uses, with a frontend Compose
  creation flow. HTTP-based `healthcheck:` stanzas translate into real
  readiness/liveness probes; `restart:` and `networks:` are parsed and
  surfaced as informational notices rather than enforced, since the
  reconciler always keeps containers running and every service in an
  app already shares one network.
- A curated ~15-entry service template catalog (ADR 015: reverses the
  original "not chasing Coolify's 280 templates" non-goal, once Compose
  support existed to build it on), served over the API and browsable
  from the creation wizard. Every template's Compose body is written
  fresh for this platform, not copied from another project's dataset.
- Git webhook receiver with HMAC-SHA256 signature verification, branch
  gating, and SHA-pinned fetch (no `git` CLI shelling).
- Persisted per-app git source and multi-app GitHub webhook support,
  with per-app HMAC secrets and PAT auth.
- GitHub App connect flow: manifest self-registration, org/repo/branch
  picker, and installation-token minting, wired in by default. GitHub
  Enterprise Server (self-hosted instances) supported alongside
  github.com. Live installation-status detection: a check against
  GitHub's API on each connection-status page load surfaces a
  suspended/revoked installation as a UI badge, falling back to the
  last known state if the check itself fails.
- Registry credentials: stored credentials for private image registries
  with a "test auth" action that validates them against the real
  registry, plus expiry tracking (healthy/expiring-soon/expired), a
  dashboard settings page, and CRUD via the API.
- GitLab (gitlab.com or self-hosted) and Bitbucket Cloud as full
  alternative git providers: OAuth connect, repo/branch picker, and
  webhook-triggered auto-deploy, the same as GitHub. A single shared
  picker component is used everywhere a repo gets chosen (the app
  creation wizard, an existing app's git-source settings), so the
  provider you connect doesn't change the mental model; GitHub's own
  webhook auto-registration degrades gracefully to a manual
  paste-the-secret banner if a pre-existing App installation predates
  the permission it now requests.
- Preview environments per pull request: opt-in per app
  (`GitSource.PreviewEnabled`, off by default). Opening or updating a
  pull request against a preview-enabled app's target branch deploys an
  independent `<app>-pr-<number>` app from the PR's head commit, reusing
  the app's own build config; closing or merging the PR tears it down
  automatically. A configured control-plane primary domain gets a
  `pr-<number>.<app>.<domain>` subdomain; without one, the preview is
  reachable by host:port like any domain-less app. GitHub only for now
  (`pull_request` events); GitLab (Merge Request Hook) and Bitbucket
  (`pullrequest:*`) webhook payloads are parsed too, but end-to-end
  testing has only covered GitHub. Wired into the dashboard (a card
  alongside git source settings: toggle, active-preview list, manual
  teardown) and the CLI (`apps previews list/teardown/enable/disable`).
  A scheduled TTL sweep also tears down any preview untouched for 7
  days by default (`APP_PREVIEW_TTL`), independent of whether a
  PR-closed webhook ever arrives, and the same sweep is callable
  on demand as an MCP tool.
- Embedded Caddy ingress with automatic TLS and domain routing. TLS
  today defaults to an internal, self-signed issuer; a public ACME
  issuer exists and is toggleable but is still unverified against a
  live domain (see In progress).
- Envelope encryption for secrets, with env injection at
  container-create time. Master-key rotation
  (`POST /api/v1/system/master-key/rotate`, `levelrail-cli secrets
  rotate-master-key`) re-wraps every stored per-service data-encryption
  key under the new master key in one transaction; rotation age is
  also checked by the doctor command below.
- Managed Redis as a first-class, volume-backed resource.
- Eight managed database engines via a dynamic engine registry
  (`internal/store/database_engines.yaml`): Postgres, Redis, MySQL,
  MongoDB, MariaDB, KeyDB, Dragonfly, and ClickHouse. Every engine gets
  resource limits, public-access toggle with host port exposure,
  scheduled/cron backups with retention, backup and restore, log search
  and live SSE log tail, metrics, generated credentials via envelope
  encryption, volume persistence across container replacement, node
  placement, and the dashboard/CLI surfaces for all of the above,
  generically across the registry rather than per-engine. Backup and
  restore is now live-Docker-tested for every engine, including
  MariaDB, ClickHouse, KeyDB, and Dragonfly's own restore paths and a
  fix scoping MongoDB restore to drop only non-system databases first.
- Restore into a brand-new, standalone resource rather than only
  in-place: `POST /api/v1/databases/{name}/restore-as-new` for managed
  databases and `POST /api/v1/apps/{name}/volumes/{volume}/restore-as-new`
  for an app's own named volume, both non-destructive to the original,
  with CLI (`restore-as-new`) and dashboard wiring.
- App volume backups: an app service's own named Docker volume gets the
  same scheduled/cron backup, restore, and verification feature set
  managed databases already had, via API, CLI
  (`app-volume-backups list/trigger/restore/restore-as-new/schedule/
  verify/verifications`), and a web UI section on the app overview page.
- Backup targets get a "test connection" action
  (`POST /api/v1/backup-targets/{id}/test`) that probes the target's
  bucket over its stored credentials without uploading or deleting
  anything, via CLI (`backup-targets test`) and a dashboard button.
- HTTP API for app CRUD and deploy trigger/history, with real
  multi-user session auth (see Dashboard and auth, below).
- Frontend app list and detail views, with a live build-log viewer and
  route-level code splitting.
- Rollback: previous images are retained, with deploy history and full
  build-log persistence. CLI `rollback` and `restart` subcommands.
- Deploy comparison: `GET /api/v1/apps/{name}/deploys/compare` diffs
  two deploy attempts' image tag and other snapshotted fields, with a
  frontend view. Env vars, ports, domains, and resource limits aren't
  snapshotted per attempt, so those aren't part of the diff yet.
- Build-failure diagnosis: a deterministic pattern matcher over a
  failed build or container's actual log text (Docker daemon down,
  image pull/auth failure, missing Dockerfile, npm/pnpm errors, port
  conflicts, readiness-probe timeout, OOM-kill), returning a matched
  signature with an excerpt and suggestion instead of just raw logs.
  Surfaced in the app overview page and as an MCP tool.
- Deploy strategies: rolling, recreate, and blue-green, plus replica
  support.
- One-shot container `exec`, gated to root-level API tokens.
- Explicit Stop/Start actions for apps, distinct from Delete/Restart:
  the reconciler tears down containers on stop and brings them back on
  start without touching desired state.
- `install.sh` (curl-pipe-sh): downloads the binary, installs Docker if
  missing, sets up a systemd unit, and verifies the control plane
  actually comes up healthy via a bounded retry loop (never an infinite
  wait) before declaring success. Safe to re-run as an upgrade path.
  Opt-in `LEVELRAIL_CONFIGURE_UFW=1` configures the host firewall
  (allow SSH before enabling, Dokku's own proven ordering), off by
  default.
- `levelrail-cli doctor`: a local preflight check (Docker daemon
  reachability, disk space, and more). The same checks also run
  server-side as a system doctor API endpoint, including a
  master-key-rotation-age check and a firewall/ufw status check, now
  with a dashboard "System status" settings page surfacing the same
  bundle, not just the CLI.
- `GET /api/v1/system/containers` and `levelrail-cli containers`: a
  read-only list of every container on the node (not just ones the
  reconciler manages), name, image, state, and ports. Deliberately no
  stop/restart from this surface, since a reconciler-managed container
  would just be recreated out from under an operator who stopped it
  here.
- Onboarding: a one-shot `/api/v1/onboarding` completion flag plus a
  frontend wizard shown on first run.
- `levelrail-cli completion bash|zsh|fish`: shell completion covering
  every command and subcommand plus the global flags.
- `levelrail-cli version` and a maintained `CHANGELOG.md`.
- Named CLI credential profiles (`--profile`/`APP_PROFILE`, AWS-CLI
  style): multiple named sections in the credentials file so one
  operator can manage several control planes without one login
  overwriting another's saved token.
- `--output json|table|text` and `--query` (JMESPath, via
  `jmespath/go-jmespath`) across the entire CLI, replacing the old
  boolean `--json` flag, which is kept as a backward-compatible alias
  for `--output json`.
- A CLI device-login flow (`levelrail-cli auth login --device`),
  RFC-8628-shaped: prints a short code and URL, an operator approves it
  from a "CLI access" dashboard settings page, and the CLI polls until
  a real API token is minted. Works over plain HTTP, since no password
  crosses the wire.
- `levelrail migrate coolify` and `levelrail migrate dokploy` CLI
  commands: pull every app off a live Coolify or Dokploy instance and
  either write app.yaml files or apply them directly to a target
  Levelrail instance.
- `levelrail apps create --interactive` (`-i`): a step-by-step wizard
  for creating an app without hand-writing app.yaml or knowing every
  flag up front, ending in either a written app.yaml or a direct API
  call, operator's choice.
- An MCP server (`cmd/levelrail-mcp`), wrapping the same versioned REST
  API and bearer-token model the CLI already uses, so a token scoped to
  fewer abilities than a tool needs gets the same 403 the REST API
  itself would return. Thirty-three tools today, across apps (list,
  get, deploy, deploy-compose, rollback, restart, status,
  deploy-history, logs, metrics), databases (list/get), nodes
  (list/get/health), service templates (list/get), feature flags
  (list/get), resource recommendations (app and database), preview
  environments (list, plus a sweep tool), alert rules (list), the audit
  log (list), deploy comparison, build-failure diagnosis, and delivery
  history for notifications, webhooks, and backup verifications (list).
  Twenty-eight of the thirty-three are read-and-suggest; the other five
  mutate something: deploy, deploy-compose, rollback, and restart for
  apps (as documented before), plus the preview sweep, which tears down
  stale preview environments on demand, the same action the scheduled
  TTL sweep above (see Preview environments) performs automatically.
  Nothing here reaches into the reconciler directly, and no tool
  deletes a resource or touches secrets/resource limits.
- Live end-to-end test suite: whole-chain push-to-HTTPS, rollback in
  both directions, the real git webhook path, database reconciliation,
  non-default port routing, node placement, protected-environment
  confirm-to-deploy gating, Compose healthcheck-derived readiness
  gating, and master-key rotation. Doesn't yet exercise multi-service
  fan-out, a full multi-node mesh, or real ACME against a live domain.

**Observability**

- Node-local metrics store at 15s resolution (CPU, memory, disk IO,
  network IO, deploy count, build duration), with configurable
  retention.
- Node-local log store with full-text search and structured (JSON) log
  parsing. Container logs are also downloadable as a file, separate
  from the live/search views.
- Federated query API across nodes, with time range, filtering, and
  aggregation.
- Frontend metrics dashboard with a range selector, historical log
  search, and deploy markers overlaid on metric charts.
- Alerting over eight rule kinds: the original threshold and crashloop
  detection, plus certificate-expiry, OS-patch-status,
  scheduled-task-failure, node-disk-space, node-resource-usage
  (per-node CPU/memory), and domain-health (a periodic DNS check
  against every domain configured on an app, catching a silently
  repointed CNAME), each with its own evaluator. Eight notification
  channel kinds: webhook, Slack, Discord, email, Telegram, Pushover,
  PagerDuty, and Microsoft Teams, plus separate deploy-outcome
  notifications. Delivery history for every notification channel is
  independently queryable via API/CLI/MCP. A dismissible dashboard
  nudge prompts enabling the platform-wide alert rules (patch-status,
  node-disk-space, node-resource-usage) when none are configured yet.
- Prometheus remote-read endpoint.
- Crashloop detection, with the last 200 lines of the failing
  container's logs surfaced automatically in the UI.
- Live app log streaming over SSE, separate from historical search.
- Node-level metrics dashboard.
- Per-node OS package-update status (a periodic collector, surfaced as
  a single current fact on the node detail page and via
  `levelrail nodes patch-status`), not an automatic patcher.
- TLS certificate renewal visibility.
- Docker disk, image, and volume pruning from the dashboard.
- Control-plane data-directory disk usage (total/free bytes) surfaced
  in Settings > General, alongside Docker's own image/volume/build-cache
  accounting.
- Managed database observability: log search, live SSE log tail, and
  metrics for every database engine, not just apps. Shares the actual
  query implementation with the app-scoped equivalents rather than a
  parallel implementation.
- Resource right-sizing: P95-based memory/CPU suggestions for apps and
  databases from observed usage over a lookback window, with a
  confidence level. A suggestion only, surfaced on the resources page;
  it never auto-applies a limit change.
- Webhook delivery history: list and manually replay past deliveries
  per git source, via API/CLI/dashboard.

**Multi-node**

- Agent transport abstraction: in-process for single-node, real gRPC
  for multi-node, with reconnection and version negotiation.
- Node registry, join-token issuance, and node CRUD.
- mTLS between control plane and agents via a minimal self-signed CA.
- Manual placement: assign or move a service to a specific node.
- WireGuard mesh with internal DNS resolving service names across
  nodes.
- Dedicated build nodes, with registry-backed remote BuildKit cache and
  per-node capability flags.
- Distributed certificate storage shared across ingress instances.
- Node health (heartbeat), cordon, and drain, plus a per-node live alert
  status (ok/firing/unknown) for the patch-status, node-disk-space, and
  node-resource-usage rule kinds, evaluated on demand for a single node
  fetch and surfaced in `nodes get`/`nodes health` and a node detail
  page card.
- Frontend node management: add-node flow, node list with health
  status, per-service node assignment, cordon and drain controls.

**Dashboard and auth**

- API tokens with scoped abilities, first-run registration, login rate
  limiting, session TTL, and an admin recovery CLI subcommand. A
  general token-bucket rate limit also applies across every
  ability-gated route beyond login, stricter for writes than reads,
  keyed per token, per session user, or per client IP if
  unauthenticated, configurable via env vars.
- Real multi-user accounts, not a single shared admin: per-user email
  sign-in or OAuth, TOTP two-factor with recovery codes, and per-user
  scoped abilities (the same ability model API tokens already had,
  now on human accounts too, so a session is no longer implicitly
  root). A full audit log records every request through the same
  ability-check hook, queryable from Settings, with CSV export. Each
  entry is attributed to a client kind (cli/dashboard/mcp/api), derived
  from the request's User-Agent header. Retention is configurable, with
  a periodic sweeper purging entries past the window plus a manual
  purge (`levelrail-cli audit-purge`, a dashboard purge button).
- Full sidebar shell with dark mode and a settings area split into
  Account, Security, General, and Tokens.
- Create App and Create Database dialogs, with health-check and
  resource-limit editors. Resource limits cover memory, CPU, swap
  (Docker's combined memory+swap ceiling, validated to never be set
  below the memory limit), and CPU pinning (`cpuset-cpus`), for both
  apps and databases.
- A settings hub with global command-palette search and a sub-sidebar
  grouping settings pages by category.
- Lightweight, non-RBAC project grouping, plus an organization tier
  above projects and an environment tier (staging/production-style
  labels) between a project and an individual app, each with its own
  shared env-var layer: organization env vars, then project env vars,
  then environment env vars, then the app's own env, in that
  override order. An environment can be marked protected, which turns
  a deploy or promotion into a 409 requiring explicit confirmation
  (a dialog in the dashboard, a confirmation prompt in the CLI).
  Environment promotion moves an app's image tag from a source
  environment to a target one; env vars, ports, domains, and resource
  limits are resolved live per environment rather than promoted, by
  design.
- Custom Docker labels escape hatch.
- App clone.
- Database public-accessibility toggle with host port exposure.
- Scheduled/cron backups to S3-compatible storage, with backup history
  and browser download of backup files (see the eight-engine backup/
  restore bullet under Core deploy path, above, including Redis's
  stop-write-start RDB reload path). Backup verification runs
  automatically right after every scheduled backup (re-download,
  re-hash, compare checksum/size/format against what was recorded at
  backup time) and can also be triggered manually; neither does a live
  restore, so verification itself is non-destructive.
- Accessibility fixes: ARIA labels on row inputs, a dialog focus-restore
  fix on the log viewer's fullscreen toggle, `aria-current` on active
  sidebar navigation, an accessible text summary for the metrics
  chart's SVG, and a primary navigation landmark.
- Dashboard "rich interactions" phase complete: the settings hub's
  command-palette search and sub-nav (above), empty states across every
  list view (apps, databases, deploy history, log search, and more)
  replaced with an icon+message+CTA pattern instead of bare "no items"
  text, keyboard navigation audited with no fixes needed (Base UI
  primitives already handle it correctly), and loading states audited
  across every route and data-fetching component: skeleton placeholders
  shaped like the eventual list/table/card content, a centered spinner
  for single-form or single-detail pages, and a progressive-rendering
  fix on the database overview page so its already-loaded header and
  status no longer wait on the Backups card's own secondary fetch.
- Environment variable editing: a row-by-row table and a raw
  paste/format "developer view" for plain vars, both listing secret
  keys inline as write-only and lockable for visibility, with actual
  secret values set through a separate Secrets card. Not a single
  unified list the way it was originally scoped, but the write-only/
  lock behavior for secrets is real.
- "Restart required" toast after saving a health-check change, with a
  one-click restart action. A resource-limit change no longer needs
  one in the common case: it applies live to any currently running
  container via the Engine API's `ContainerUpdate`, falling back to
  the same restart-required toast only when no container is running
  yet or the live update itself fails. Env changes don't trigger it
  either way.
- Named roles: three curated ability-set presets (admin, operator,
  viewer, `internal/api/roles.go`) applied to a user in one action via
  `role` on create/update, instead of hand-picking abilities one at a
  time. `GET /api/v1/roles` lists the presets; the users settings page's
  create/edit dialogs get a role dropdown that falls back to "Custom"
  for a hand-picked set; the CLI's `users create`/`users set-abilities`
  take a `--role` flag as an alternative to `--abilities`.
- An IAM-style, resource-scoped policy engine
  (`internal/api/iam.go`, `internal/store/iam_policy.go`): JSON policy
  documents with AWS-IAM-shaped Allow/Deny statements (`Effect`,
  `Action` as ability strings or `*`, `Resource` as identifiers like
  `app:myapp` or `*`), attachable to a user or an API token, additive
  on top of that principal's existing flat abilities list rather than
  replacing it. Enforcement is wired into `GET/PUT/DELETE
  /api/v1/apps/{name}` and `GET/DELETE /api/v1/databases/{name}` via a
  `requireAbilityForResource` middleware. CLI:
  `levelrail-cli iam policies create/list/get/update/delete/attach/
  detach/attachments`. Dashboard: an "IAM policies" settings page.
- Feature flags: a boolean plus an optional gradual rollout percentage,
  scoped to an app, read live at runtime by the app's own code via
  `GET /api/v1/flags/evaluate/{key}` and a read-scoped API token, no
  redeploy or restart involved since the value is never baked into a
  container. Full CRUD under `/api/v1/apps/{name}/flags`, a dashboard
  tab with live toggle and rollout controls, and a `flags` CLI command
  group. See `docs/feature-flags.md`.

## In progress

- **Database backup-schedule UI.** Shipped (`BackupScheduleForm`,
  see Done); this line is kept only as a pointer in case a gap
  surfaces on real use.
- **Multi-service apps.** Backend fan-out is real: an `apps` table
  links N desired services under one app, and
  `POST /api/v1/apps/{name}/deploy-spec` fans a `services:` map out
  into independent per-service builds and deploys, each tracked
  separately, with a real frontend (a services tab plus
  `DeploySpecForm`) driving it. Webhook-triggered auto-deploy is now
  unified with that same path: a git source can persist a `services:`
  map (`GitSource.Services`), and a push fans out through the identical
  `DeploySpec` logic instead of the older, simpler
  `GitSource.AdditionalServices` flat list. A git source with no
  `services:` map still falls back to `AdditionalServices` unchanged,
  so existing single-service webhook setups are unaffected.
  `apps create --interactive` now also supports it: an "add another
  service?" loop after the first service's answers, writing every
  service into one `app.yaml` in file mode or fanning out through
  `deploy-spec` (instead of `CreateApp`) in API mode. Kept here rather
  than moved to Done because the live end-to-end suite (see Done,
  above) doesn't yet exercise multi-service fan-out.
- **Real public ACME.** The Caddy ACME issuer type, a settings toggle,
  and form validation are all built and wired end to end (Settings >
  Domains). Only unit-tested against the config shape so far, not
  spot-checked against a real domain issuing a real cert, the exact
  gap ADR 005 named at Phase 0.

## Not started

Nothing currently on this list: preview environments (this page's own
"Done" section, above) and live in-place resource-limit application
were the last two items here, and both have since shipped.

## Explicitly out of scope

Pulled directly from the project's own non-goals:

- Not building a container runtime. Docker Engine API only.
- Not building a scheduler with bin-packing, affinity rules, or
  autoscaling in v1.
- Not building a service mesh. WireGuard plus DNS is enough.
- Not supporting Windows or non-Linux nodes.
- Not building a managed cloud offering until the self-hosted version
  has real users.
- No AI in the reconciliation path. AI is a read-and-suggest layer on
  top of the API, nothing more.
- No Kubernetes compatibility layer, no CRDs, no custom orchestration
  standard. Borrow the patterns, skip the ecosystem.
