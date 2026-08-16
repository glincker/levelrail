# Roadmap

Status as of 2026-08-16, not the aspirational plan. See CLAUDE.md in the
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
- Static-site builds (`build.type: static`), though without a frontend
  surface yet to configure them.
- Git webhook receiver with HMAC-SHA256 signature verification, branch
  gating, and SHA-pinned fetch (no `git` CLI shelling).
- Persisted per-app git source and multi-app GitHub webhook support,
  with per-app HMAC secrets and PAT auth.
- Embedded Caddy ingress with automatic TLS and domain routing. TLS
  today comes from an internal, self-signed issuer, not a public ACME
  provider (see Not started).
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
  restore (Postgres and MySQL live-verified, Redis restore unsupported
  by design), and browser download of backup files.
- Accessibility fixes (ARIA labels on row inputs).

## In progress

- **Managed Postgres.** Further along than earlier notes suggested: the
  reconciler is wired to secrets management and backups are
  live-verified, but full parity with Redis and MySQL hasn't been
  confirmed.
- **Dashboard "rich interactions" phase**: command palette, richer
  empty/loading states, keyboard navigation. Not yet detailed or
  dispatched.
- **GitHub App/OAuth connect flow.** Fully built (manifest
  self-registration, org/repo/branch picker, installation-token
  minting), but sitting in an open, unmerged, conflicting pull request.
  Not shipped until merged.
- **Unified env var UI**: a single list combining plain and
  secret-flagged env, with a permanent non-revealable lock for secrets.
  Dispatched, not yet reviewed.
- **"Restart required" toast** after saving env/health/resource
  changes. QA-confirmed, not yet merged.
- **Per-node disk space metric.** Built, but has no observable value in
  single-node mode. A follow-up to surface local disk via system status
  is dispatched, not yet reviewed.
- **Database backup-schedule UI.** The backend route exists; the
  frontend is parked pending a gap-scan review.
- **Multi-service apps.** A design proposal for the real fix (a
  dedicated apps table, per-service partial-failure deploys) has
  landed, but implementation hasn't started, pending a decision on two
  open questions.
- **Coolify/Dokploy migration tooling.** Design proposal landed,
  awaiting review, no code written.
- **Install/upgrade packaging** (a curl-pipe-sh installer plus a
  systemd unit). Research and a recommendation have landed, awaiting
  sign-off, nothing built yet.

## Not started

- Real public ACME certificates, issued against a live domain rather
  than the current internal, self-signed issuer.
- Multi-service apps end-to-end. Every app today is exactly one
  container. The app-spec schema allows a services map, but the deploy
  path only ever drives a single service.
- Team/multi-user access, RBAC, and an audit log. There's a hard single
  admin user, API tokens scope abilities rather than human users, and
  no audit log exists anywhere.
- Preview environments per pull request. Depends on multi-service
  support landing first.
- An install and upgrade path for the product itself (see In progress
  for the packaging work already underway).
- Redis backup restore. Returns an explicit "not supported" error by
  design, not a silent gap.
- An explicit "Stop" action for apps, distinct from Delete/Restart.
- Docker Compose as a deploy target, a template catalog, teams, and the
  MCP server.
- Live, in-place resource-limit application without a restart. A saved
  health-check or resource-limit change doesn't reconcile into the
  already-running container until a restart forces a new one (the
  "restart required" toast above is the in-flight fix).

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
