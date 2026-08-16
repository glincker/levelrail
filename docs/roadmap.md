# Roadmap

Status as of 2026-08-16 (refreshed against current `main`), not the
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
- Git webhook receiver with HMAC-SHA256 signature verification, branch
  gating, and SHA-pinned fetch (no `git` CLI shelling).
- Persisted per-app git source and multi-app GitHub webhook support,
  with per-app HMAC secrets and PAT auth.
- GitHub App connect flow: manifest self-registration, org/repo/branch
  picker, and installation-token minting, wired in by default.
- Embedded Caddy ingress with automatic TLS and domain routing. TLS
  today defaults to an internal, self-signed issuer; a public ACME
  issuer exists and is toggleable but is still unverified against a
  live domain (see In progress).
- Envelope encryption for secrets, with env injection at
  container-create time.
- Managed Redis as a first-class, volume-backed resource. Managed
  Postgres also exists (see In progress for scope caveats).
- MySQL as a third database engine, via a dynamic engine registry.
- HTTP API for app CRUD and deploy trigger/history, with a single admin
  user and session auth. No teams or RBAC yet.
- Frontend app list and detail views, with a live build-log viewer and
  route-level code splitting.
- Rollback: previous images are retained, with deploy history and full
  build-log persistence. CLI `rollback` and `restart` subcommands.
- Deploy strategies: rolling, recreate, and blue-green, plus replica
  support.
- One-shot container `exec`, gated to root-level API tokens.
- Explicit Stop/Start actions for apps, distinct from Delete/Restart:
  the reconciler tears down containers on stop and brings them back on
  start without touching desired state.
- `install.sh` (curl-pipe-sh) and a systemd unit for the control plane
  itself, safe to re-run as an upgrade path.
- `levelrail migrate coolify` CLI command: pulls every app off a live
  Coolify instance and either writes app.yaml files or applies them
  directly to a target Levelrail instance.
- Live end-to-end test suite: whole-chain push-to-HTTPS, rollback in
  both directions, the real git webhook path, database reconciliation,
  non-default port routing, and node placement. Doesn't yet exercise
  multi-service fan-out, a full multi-node mesh, or real ACME against
  a live domain.

**Observability**

- Node-local metrics store at 15s resolution (CPU, memory, disk IO,
  network IO, deploy count, build duration), with configurable
  retention.
- Node-local log store with full-text search and structured (JSON) log
  parsing.
- Federated query API across nodes, with time range, filtering, and
  aggregation.
- Frontend metrics dashboard with a range selector, historical log
  search, and deploy markers overlaid on metric charts.
- Threshold alerting over metrics, with webhook, Slack, Discord, email,
  and Telegram notification channels, plus separate deploy-outcome
  notifications.
- Prometheus remote-read endpoint.
- Crashloop detection, with the last 200 lines of the failing
  container's logs surfaced automatically in the UI.
- Live app log streaming over SSE, separate from historical search.
- Node-level metrics dashboard.
- TLS certificate renewal visibility.
- Docker disk, image, and volume pruning from the dashboard.
- Control-plane data-directory disk usage (total/free bytes) surfaced
  in Settings > General, alongside Docker's own image/volume/build-cache
  accounting.

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
- Node health (heartbeat), cordon, and drain.
- Frontend node management: add-node flow, node list with health
  status, per-service node assignment, cordon and drain controls.

**Dashboard and auth**

- API tokens with scoped abilities, first-run registration, login rate
  limiting, session TTL, and an admin recovery CLI subcommand.
- Full sidebar shell with dark mode and a settings area split into
  Account, Security, General, and Tokens.
- Create App and Create Database dialogs, with health-check and
  resource-limit editors.
- Lightweight, non-RBAC project grouping.
- Custom Docker labels escape hatch.
- App clone.
- Database public-accessibility toggle with host port exposure.
- Scheduled/cron backups to S3-compatible storage, with backup history,
  restore (Postgres, MySQL, and Redis all live-verified, including
  Redis's stop-write-start RDB reload path), and browser download of
  backup files.
- Accessibility fixes (ARIA labels on row inputs).
- Environment variable editing: a row-by-row table and a raw
  paste/format "developer view" for plain vars, both listing secret
  keys inline as write-only and lockable for visibility, with actual
  secret values set through a separate Secrets card. Not a single
  unified list the way it was originally scoped, but the write-only/
  lock behavior for secrets is real.
- "Restart required" toast after saving a health-check or
  resource-limit change, with a one-click restart action. Env changes
  don't trigger it.

## In progress

- **Managed Postgres.** Further along than earlier notes suggested: the
  reconciler is wired to secrets management and backups are
  live-verified, but full parity with Redis and MySQL hasn't been
  confirmed.
- **Dashboard "rich interactions" phase.** Command palette, richer
  empty/loading states, keyboard navigation. Not yet detailed or
  dispatched.
- **Database backup-schedule UI.** The backend route exists; the
  frontend is parked pending a gap-scan review.
- **Multi-service apps.** Backend fan-out is real: an `apps` table
  links N desired services under one app, and
  `POST /api/v1/apps/{name}/deploy-spec` fans a `services:` map out
  into independent per-service builds and deploys, each tracked
  separately. The git webhook handler auto-fans-out too, when a
  `Services` map is configured on it. But the actual per-app
  git-source webhook path used for real connected repos still looks
  up exactly one service, and there's no frontend for any of this. Not
  end-to-end yet.
- **Dokploy migration tooling.** Coolify migration shipped (see Done);
  Dokploy has no code written yet.
- **Real public ACME.** The Caddy ACME issuer type, a settings toggle,
  and form validation are all built and wired end to end (Settings >
  Domains). Only unit-tested against the config shape so far, not
  spot-checked against a real domain issuing a real cert, the exact
  gap ADR 005 named at Phase 0.

## Not started

- Team/multi-user access, RBAC, and an audit log. There's a hard single
  admin user, API tokens scope abilities rather than human users, and
  no audit log exists anywhere.
- Preview environments per pull request. Depends on multi-service
  support landing end-to-end first.
- Docker Compose as a deploy target, a template catalog, and the MCP
  server.
- Live, in-place resource-limit application without a restart. A saved
  health-check or resource-limit change doesn't reconcile into the
  already-running container until a restart forces a new one (the
  shipped "restart required" toast is the operator-facing workaround,
  not a fix to the underlying gap).

## Explicitly out of scope

Pulled directly from the project's own non-goals:

- Not building a container runtime. Docker Engine API only.
- Not building a scheduler with bin-packing, affinity rules, or
  autoscaling in v1.
- Not building a service mesh. WireGuard plus DNS is enough.
- Not chasing Coolify's 280 one-click templates. Ten good ones beat 280
  stale ones.
- Not supporting Windows or non-Linux nodes.
- Not building a managed cloud offering until the self-hosted version
  has real users.
- No AI in the reconciliation path. AI is a read-and-suggest layer on
  top of the API, nothing more.
- No Kubernetes compatibility layer, no CRDs, no custom orchestration
  standard. Borrow the patterns, skip the ecosystem.
