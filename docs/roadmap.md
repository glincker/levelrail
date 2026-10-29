---
title: Roadmap
description: What Levelrail has shipped by area, what is still open, and what is out of scope.
"og:url": "https://docs.levelrail.com/roadmap"
---

Releases are listed on the [releases page](https://github.com/glincker/levelrail/releases). "Shipped" on this page means built and merged, not proven in production. For per-feature maturity and the evidence behind each label, see [feature status](/feature-status). Release-by-release detail is in the [changelog](/changelog). The decisions behind the build order are in the [ADRs](https://github.com/glincker/levelrail/tree/main/adr).

The 0.8 release line adds an Inbox for alerts and approvals, recoverable upgrades, secret rotation, and provider, region and zone labels on nodes.

The build did not follow the original phase plan in order. Parts of multi-node shipped while some single-node items (real public ACME renewal, a Linux idle-footprint measurement) are still open. The status sections below describe what is true today. The dated section above is a plan, not a schedule.

## Now, next, later

Last updated 2026-10-29. There is no stable release yet. Every release so far is a beta. Everything below is planned, not promised: quarters are targets, items can move or drop, and nothing here is a commitment to a date. Items marked planned have no shipped code unless the line says so.

### Now (Q4 2026)

Goal: make the beta claim publishable. Reliability and evidence first.

- Real hosts: record real-host runs for the multi-node WireGuard mesh and cross-host transport, and a bare metal or VPS idle-footprint number. Status: open, see [Open work](#open-work).
- Public ACME: renewal close to expiry, DNS-01 wildcards and a deliberate failure against the production CA. Status: open, one live run is recorded in the [runbook](/acme-verification-runbook).
- Upgrade and downgrade testing across more releases with real data. Status: started, see [Upgrade safely](/upgrade-safely).
- Stacks hardening: stacks are experimental behind `APP_EXPERIMENTAL=iac`, with drift, approvals and rollback built. Status: shipping in the 0.8 line, see the [changelog](/changelog) and [ADR 042](https://github.com/glincker/levelrail/blob/main/adr/042-stacks-and-revisions.md).
- Public live demo and Codespaces: a read-only demo you can try without installing, and a Codespaces config for a one-click dev setup. Status: planned. See [demo](/demo) for what exists today.
- Community and governance files: a code of conduct, contributing guide and security policy exist in `.github`. Still planned: a governance document and a maintainer and review policy.

### Next (Q1 2027)

Goal: fewer steps between a first deploy and a repeatable setup.

- Release snapshots and environment bindings: pin a set of app versions and settings as a snapshot, and bind it to an environment so promotion moves a known set. Status: planned, builds on environment promotion and cloning.
- Schema-driven config scaffold: generate starter `app.yaml` and stack files from the embedded JSON Schema, so field names and types come from one source. Status: planned, builds on `stack init` and the [app spec reference](/app-spec-reference).
- MCP audit coverage: every MCP tool call, including read-only ones, shows up in the audit log with the token and the tool name, and a test fails when a new tool skips it. Status: planned. See [MCP tool surface](/mcp-tool-surface).
- Template coverage: deploy more of the roughly 360 templates in e2e tests. Status: open.
- Load and failover numbers: publish the first results from `levelrail-cli bench run`. Status: harness exists, no published results, see [benchmarks](/benchmarks).

### Later (Q2 2027)

Goal: widen what is proven, only after the earlier goals are met.

- Read-only incident and cost analysis agent: reads metrics, logs, deploys and spend data and writes a summary with suggested next steps. It is opt-in, off by default, and never changes anything; it stays a read-and-suggest layer, see [out of scope](#out-of-scope). Status: planned.
- Database coverage: point-in-time restore and TLS for more engines than today. Status: open, depends on what each engine supports.
- Real pause or scale-to-zero for spend caps: needs an accepted ADR first, see [ADR 041](https://github.com/glincker/levelrail/blob/main/adr/041-spend-pause-scale-to-zero.md). Status: design.
- Control-plane high availability: see [ADR 037](https://github.com/glincker/levelrail/blob/main/adr/037-control-plane-ha-replication.md), [ADR 039](https://github.com/glincker/levelrail/blob/main/adr/039-control-plane-ha-wal-shipping-vs-postgres.md) and [ADR 047](https://github.com/glincker/levelrail/blob/main/adr/047-warm-standby-control-plane-promote-drill.md). Status: design and drill, not a release goal yet.
- A first stable release: only when Phase 5 evidence (upgrade testing, real-host runs, a published footprint number) is in. No date is set.

## Phase status

```mermaid
flowchart LR
  P0["Phase 0: Foundation<br/>Shipped"] --> P1["Phase 1: Single node<br/>Shipped"]
  P1 --> P2["Phase 2: Observability<br/>Shipped"]
  P2 --> P3["Phase 3: Multi-node<br/>Shipped, beta"]
  P3 --> P4["Phase 4: Platform surface<br/>Mostly shipped"]
  P4 --> P5["Phase 5: Hardening<br/>Open"]
```

Phase 5 (chaos testing, upgrade and downgrade testing across versions, security review, a published idle-footprint number, a shadow-running period) is the open work. Some of it has started: control-plane death survival, break-glass agent reconnect and a first live ACME run are covered, see [resilience](/resilience) and the [ACME runbook](/acme-verification-runbook). Upgrade testing has started too: a harness upgrades the real v0.8.1 and v0.8.2 binaries with seeded data and checks restore points, automatic restore and the downgrade refusal (see [Upgrade safely](/upgrade-safely#upgrading-real-releases-with-real-data)); a second test upgrades v0.8.2 data across real migrations (email providers backfill, spend cap guard mode, the migration list, the downgrade refusal and restores), see [Cross-release schema migration](/upgrade-safely#cross-release-schema-migration).

## Shipped

### Deploy path

- App spec (`app.yaml` or `deploy.yaml`) parsing with an embedded JSON Schema. See the [app spec reference](/app-spec-reference).
- Docker Engine API wrapper, with no `docker` CLI shelling.
- Application reconciler with readiness probes gating every cutover and liveness probes on each pass. A container that OOM-kills or exits during the readiness wait fails immediately (`OOMKilledDuringReadiness`, `ExitedDuringReadiness`) instead of retrying a dead address.
- BuildKit builds with live build-log streaming over SSE. Railpack auto-detection, static sites (`build.type: static`), and dedicated [build nodes](/build-node-routing) with a registry-backed cache (a built-in registry can be enabled in Settings).
- Docker Compose as a deploy target for a supported subset of `compose.yaml`, and multi-service apps (`deploy-spec`) with per-service builds and deploys.
- Deploy strategies: rolling, recreate, blue-green. Rollback to retained images, deploy history, deploy comparison, and deterministic build-failure diagnosis.
- Pre and post-deploy hooks. A failing pre-deploy hook blocks cutover and keeps the old container serving.
- Git providers: GitHub (App flow, including GitHub Enterprise Server), GitLab, Bitbucket Cloud and Gitea, with HMAC-verified webhooks and webhook delivery history with replay. See [git integrations](/git-integrations).
- [Preview environments](/previews) per pull request (opt-in), with optional ephemeral databases and PR status comments. See [deploy previews](/deploy-previews).
- Embedded Caddy ingress: automatic TLS (internal issuer by default, public ACME toggleable), zero-config `sslip.io` hostnames, per-domain bring-your-own certificates. See [domains and ingress](/domains-and-ingress).
- Envelope-encrypted secrets with master-key rotation. See [security](/security).
- Container exec and an interactive terminal (dashboard or `apps exec --interactive`), gated to root-level tokens.
- [Service template](/templates-and-registry) catalog: about 360 entries (see [template catalog](/template-catalog)), reversing the original non-goal in [ADR 015](https://github.com/glincker/levelrail/blob/main/adr/015-service-template-catalog-reversal.md).
- Ingress abuse defense with per-app rate limits and address and User-Agent deny lists, starting in log only mode with a replay before enforcing. See [ingress abuse defense](/abuse-defense).
- A chaos test matrix that maps daemon, disk, agent, image and OOM failures to the test that proves each one converges. See [chaos test coverage](/chaos-coverage).

### Data

- Nine managed database engines: Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB, Valkey, Dragonfly and ClickHouse, with scheduled backups, restore, verification, metrics and log search. Meilisearch, Qdrant, Typesense and Memcached are built but not yet verified on a live Docker daemon. See [managing databases](/managing-databases).
- Point-in-time restore for Postgres (opt-in WAL archiving, restore to a timestamp inside the recoverable window) and for MySQL and MariaDB (opt-in binary log archiving, restore into a new database), restore into a new resource, and app volume backups to S3-compatible targets. See [backups and storage](/backups-and-storage).
- TLS on newly created Postgres, Redis, MySQL, MariaDB and MongoDB databases. Databases created before this feature are not switched.
- Control-plane backups. See [control-plane backup](/control-plane-backup).

### Observability

- Node-local metrics (15s resolution) and log store with full-text and JSON search, federated across nodes, and a Prometheus remote-read endpoint. Deploy markers appear on metric charts.
- Crashloop detection with the last 200 log lines surfaced in the UI.
- Alerting with 25 rule kinds and 18 notification channel kinds, with retries for transient failures and delivery history.
- Attention list, [status page](/status-page), capacity forecast, right-sizing suggestions and certificate renewal visibility. See [observability](/observability).
- Email providers with a primary and an optional backup that takes over automatically, provider templates, branding, and a credential reveal behind a password and two-factor check. See [email notifications](/email-notifications).
- Monthly egress and cost caps per app and per project, with alerts at 50, 80 and 100 percent, a deploy guard with an audited override, a month-end forecast and a pause preview. See [spend caps](/spend-caps).
- Traffic export of one record per finished ingress request to PostHog, OpenTelemetry or a webhook, with no visitor identity unless a destination opts in. See [traffic export](/traffic-export).

### Multi-node

- Agent transport: in-process for a single node, gRPC with mTLS for remote nodes, with reconnection and version negotiation.
- Node registry, join tokens, cordon, drain, heartbeats and connection history.
- Guided add-node flow with provider, region and zone auto-detect from cloud metadata, preflight checks with fixes, and provider-aware spread (beta, see [add a node anywhere](/add-node-anywhere)).
- Manual placement, moving an app with its volumes, WireGuard mesh with cross-node service DNS, and distributed certificate storage.

The join flow is verified across two real Docker daemons, but not across a real WAN. The mesh itself is not verified on real hosts. See [multi-node](/multi-node).

### Identity and access

- Multi-user accounts with TOTP and recovery codes, OIDC sign-in, scoped API tokens, named roles (admin, operator, viewer), and IAM-style resource-scoped policies. See [identity and access](/identity-and-access).
- Organizations, projects and environments with layered shared env vars, protected environments, two-person deploy approvals, environment promotion and cloning.
- Audit log with search, CSV export and retention.
- Team invites, rate limiting, security headers (HSTS is opt-in).
- A theauth-library auth path exists behind default-off flags. See [auth engine](/auth-engine).

### Operator surfaces

- Dashboard, `levelrail-cli` (profiles, `--output` and `--query`, shell completion, device login, `doctor`, `apps create --interactive`) and `install.sh`.
- Migration CLI: `levelrail-cli migrate coolify`, `dokploy` and `caprover`. See [migrating from Coolify, Dokploy and CapRover](/migrating-from-coolify-dokploy-and-caprover).
- MCP server (`cmd/levelrail-mcp`): over 240 tools, grouped into toolsets that an agent enables on demand. See [MCP tool surface](/mcp-tool-surface) for the exact, generated numbers, and the [AI assistant](/ai-assistant) page. Tokens are checked against the same abilities as the REST API.
- `migrate verify`, a parity report that compares the source with this server before DNS moves, shown on the Cutover tab of the Migrate page. See [migration reference](/migration-reference).
- Cross-release schema migration tests that upgrade real v0.8.2 data across real migrations, with downgrade refusal and automatic restore. See [upgrade safely](/upgrade-safely).
- The dashboard Start tab follows the project and environment picker. See [projects, organizations and environments](/projects-and-organizations).

### Shipped but off by default or beta

These exist and are tested, but are gated or carry a beta label. See [feature status](/feature-status) and [experimental features](/experimental-features).

- In-app AI chat, AI models on GPU nodes, [load balancer](/load-balancing), [platform as code](/platform-as-code), Cloudflare Tunnel (all behind `APP_EXPERIMENTAL`).
- Pipelines, feature flags, deploy freeze, supply chain scanning, [log archive](/log-archive), [scheduled deploys](/scheduled-deploys), deploy cost estimate.

## Open work

- Real public ACME: one live run against Let's Encrypt on a public VPS is recorded in the [runbook](/acme-verification-runbook). Renewal close to expiry, DNS-01 wildcards and a deliberate failure against the production CA are not yet exercised live.
- Multi-node on real hosts: the WireGuard mesh and cross-host remote transport are unverified outside fakes and local Docker daemons.
- Idle footprint on Linux: measured in a Docker Desktop container (not bare metal) with a harness and opt-in regression guard. A bare metal or VPS number is still open. See [performance](/performance).
- Load and failover numbers: the harness exists (`levelrail-cli bench run`), no published results yet. See [benchmarks](/benchmarks).
- Template coverage: a sample of the roughly 360 templates is deployed by e2e tests, not the whole catalog.
- Other database engines: PITR covers Postgres, MySQL and MariaDB only (MongoDB has no oplog here), and the remaining engines have no TLS by default.
- Stable release: none yet. Upgrade and downgrade testing across versions with real data is part of Phase 5.
- Real pause or scale-to-zero for spend caps: `pause` serves a maintenance response and does not stop containers. A real pause needs an ADR first.
- Remote Go build cache for CI: needs a storage bucket before it can be enabled.

## Out of scope

- Container runtime: Docker Engine API only.
- Scheduler: no bin-packing or affinity rules. Resource-aware placement, failure-domain spread and autoscaling are built, see [ADR 033](https://github.com/glincker/levelrail/blob/main/adr/033-scale-out-roadmap-and-non-goal-reversal.md) and [ADR 036](https://github.com/glincker/levelrail/blob/main/adr/036-autoscaler-dry-runs-and-multi-ingress-readiness.md).
- Service mesh: WireGuard plus DNS only.
- Windows or non-Linux nodes.
- Managed cloud offering: not before the self-hosted version has real users.
- AI in the reconciliation path: AI is a read-and-suggest layer on top of the API.
- Kubernetes compatibility: no CRDs, no custom orchestration standard.

## See also

<CardGroup cols={2}>
<Card title="Feature status" href="/feature-status">

Maturity labels and the evidence behind each one.

</Card>
<Card title="Multi-node deployment" href="/multi-node">

Add and manage more than one server.

</Card>
</CardGroup>
