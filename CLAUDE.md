# Project Plan: Levelrail (Self-Hosted Deployment Platform)

Status: Phase 0, active
Owner: Gagan (thegdsks / GLINCKER)
Audience: this file is the root context document for Claude Code sessions. Read it before any implementation work.

## Naming decision (2026-08-11)

The public name is **Levelrail**, decided after four rounds and 65 candidates of generation plus live registry verification (npm, crates.io, PyPI, Docker Hub, GitHub, Homebrew, AUR, Wikipedia, App Store, whois). Full trail is in `docs-local/research/naming/`.

Levelrail was the only candidate clean across every check across all four rounds, including the `.com`. It also encodes a real architectural decision: the reconciler is level-triggered, never edge-triggered (see 4.2). The known cost is a phonetic echo of Railway, a direct competitor — accepted knowingly, not overlooked.

**This does not change the rule below.** "Levelrail" is documentation, not code. It appears in this file, in ADRs, in the git remote name, and in `brand.yaml`. It must never appear as a hardcoded string anywhere under `/cmd`, `/internal`, `/web`, or `/proto`. The brand indirection layer in section 3 is not a formality now that naming is settled — it's exactly as load-bearing as before.

## 1. What this is

A self-hosted platform that turns one or more Linux boxes into a private cloud. Push to a git repo, get a running app with TLS, logs, metrics, and rollback. Single Go binary for the control plane, single Go binary for the node agent, Docker as the container runtime.

The reason to build it rather than use Coolify or Dokploy is architectural, not cosmetic:

1. Both of them drive remote servers by SSHing in and shelling out `docker` CLI commands, then parsing text output. That is the source of most flakiness in this category and most of the idle CPU burn, because it forces polling loops.
2. Neither treats observability as a first-class primitive. Metrics and logs are bolted on, usually by telling the user to install Grafana as a one-click app.
3. Dokploy is built on Docker Swarm, which has been in maintenance mode for years. Coolify is a Laravel monolith with a Livewire frontend that degrades with resource count.

So the wedge is: agent-based control plane, observability built into the core, low idle footprint that stays low as the number of managed apps grows.

Not a Kubernetes competitor. Not a Vercel competitor. The target user runs between 3 and 50 services on between 1 and 10 machines and does not want to learn Kubernetes.

Positioning shorthand for this repo: a lightweight, AI-ready Coolify alternative. "AI-ready" means the HTTP API is designed from Phase 1 so the Phase 4 MCP layer bolts on cleanly (see 4.11) — it does not mean AI enters the reconciliation path. That rule (4.2, 4.11) is unchanged by the positioning language.

## 2. Non-goals

Writing these down because scope creep kills projects in this category.

- Not building a container runtime. Docker Engine API only.
- Not building a scheduler with bin-packing, affinity rules, or autoscaling in v1.
- Not building a service mesh. WireGuard plus DNS is enough.
- Not chasing Coolify's 280 one-click templates. Ten good ones beat 280 stale ones.
- Not supporting Windows or non-Linux nodes.
- Not building a managed cloud offering until the self-hosted version has real users.
- No AI in the reconciliation path. AI is a read-and-suggest layer on top of the API, nothing more.
- No Kubernetes compatibility layer, no CRDs, no custom orchestration standard. Borrow the patterns, skip the ecosystem.

## 3. Naming and rebrandability

**Rule: no product name string appears anywhere in source code, ever.** This held true through four rounds of naming and stays true now that the name is decided — see the naming decision note above.

Implementation:

- `internal/brand/brand.go` holds a single `Brand` struct: `Name`, `ShortName`, `BinaryName`, `Domain`, `SupportURL`, `PrimaryColor`, `LogoSVG`, `DocsURL`.
- Values load from `brand.yaml` at build time, overridable at runtime by `APP_BRAND_*` env vars. (Deliberately not `LEVELRAIL_BRAND_*` — an env prefix that bakes in the current name would itself need a migration if the name ever changes, defeating the point.)
- Go binary name comes from an ldflags variable, not from the directory name.
- Frontend reads brand from a `/api/v1/brand` endpoint on boot, hydrates a React context. No hardcoded strings in components.
- All filesystem paths derive from a single `DataDir` constant, default `/var/lib/levelrail-data` pending final confirmation, env override `APP_DATA_DIR`.
- Docker labels, network names, and volume prefixes use `brand.ShortName` as the namespace.
- CLI command name comes from `os.Args[0]`, so renaming the binary renames the command.
- Add a CI check that greps the source tree for candidate name strings and fails the build if found.

Do the same for config. The app spec file the user writes in their repo should be discovered by a list of candidate filenames (`app.yaml`, `deploy.yaml`, plus the branded one) so a rename does not break existing users later.

## 4. Locked architecture decisions

These are settled. Do not relitigate them in agent sessions without an explicit ADR.

### 4.1 Control plane: Go, single static binary

Embedded frontend assets via `embed.FS`. No Node runtime on the server. Chi or stdlib `net/http` with Go 1.22+ routing for the HTTP layer. Do not pull in a heavy framework.

### 4.2 Orchestration: custom reconciler over Docker Engine API

Rejected: Swarm (maintenance mode), Nomad (BSL license), K3s (defeats the purpose, brings the whole Kubernetes surface back).

The pattern is borrowed from Kubernetes controllers without the runtime:

- Desired state lives in the database as declarative resource records.
- A reconcile loop per resource type diffs desired against observed and converges.
- Observed state comes from Docker event streams, not polling.
- Reconcilers are idempotent, level-triggered, and safe to interrupt. Never edge-triggered.
- Every reconcile emits a status condition with a reason string, stored and shown in the UI.

This is the core of the product. Roughly 3000 lines of Go done properly. Get it right before anything else.

### 4.3 Node communication: reverse-dialed gRPC agent with mTLS

The agent runs on every managed node and dials **out** to the control plane. Consequences:

- No inbound ports open on managed servers.
- Works behind NAT and residential connections.
- Agent streams Docker events up rather than the control plane polling down.
- Agent talks to the local Docker socket via the Engine API directly. No shelling out to the `docker` CLI, ever.

Enrollment: control plane issues a one-time join token, agent exchanges it for a client certificate. Certificate rotation on a schedule.

The agent must also work in single-node mode where it runs in the same process as the control plane, communicating over an in-memory transport that implements the same interface. This keeps the code path identical whether there is one node or ten.

### 4.4 Builds: BuildKit as a library

Use `moby/buildkit`'s Go client, not `docker build`. This gives remote build cache, parallel stage execution, and cache mounts. Remote cache is the difference between a 6 minute Next.js build and a 40 second one, and neither competitor exposes it well.

Auto-detection: Railpack, not Nixpacks. Railpack is the successor and is significantly lighter. Support Dockerfile and Compose as first-class inputs. Static sites get served by the embedded Caddy directly with no container.

### 4.5 Ingress: Caddy embedded as a Go library

Import `caddyserver/caddy/v2` into the control plane binary and drive it through the in-process admin API. ACME certs, HTTP/3, and automatic renewal come free. Rejected Traefik because it means a separate container with its own config surface, which breaks the single-binary story.

Certificate storage goes in the database so multi-node deployments share cert state.

### 4.6 Node networking: WireGuard mesh

Replaces the Swarm overlay network. Each node gets a peer, the control plane distributes peer config. Apps get stable internal DNS names that resolve across machines. Use `wireguard-go` for portability, with kernel module detection for the fast path.

Do not build this in Phase 1. Do design the network abstraction in Phase 1 so it can be swapped in.

### 4.7 State: embedded SQLite in WAL mode

Control plane write volume is tiny. SQLite keeps the single-binary story honest. Use `modernc.org/sqlite` (pure Go, no cgo) so cross-compilation stays trivial.

Schema migrations with an embedded migration tool, versioned and forward-only. Add a Postgres driver in a later phase only if HA control plane becomes a real request.

### 4.8 Observability: node-local storage, federated query

This is the differentiator. Do not ship all telemetry to a central store, because that is exactly what makes these platforms heavy.

- Each agent keeps a local time-series store for container metrics. Evaluate embedded VictoriaMetrics vs a purpose-built ring buffer. Retention configurable, default 15 days at 15s resolution.
- Each agent keeps a local log store with full-text search. Read Docker's log stream, chunk, compress, index. Do not use the json-file driver's files directly.
- The control plane fans queries out to agents and merges results. Query is pull, not push.
- Idle cost stays near zero because nothing is being shipped or centrally indexed.
- Expose a Prometheus-compatible remote read endpoint so people can point Grafana at it if they want.

Metrics that must exist per app without configuration: CPU, memory, disk IO, network IO, request rate, response time percentiles, error rate, container restart count, build duration, deploy frequency.

### 4.9 App spec: one declarative file in the repo

The user-facing contract. Keep it small enough to hand-write.

```yaml
version: 1
services:
  web:
    build:
      type: dockerfile        # dockerfile | compose | railpack | static
      path: ./Dockerfile
    domains:
      - app.example.com
    port: 3000
    health:
      readiness: { path: /healthz, interval: 5s, timeout: 2s }
      liveness:  { path: /healthz, interval: 30s, failures: 3 }
    resources:
      memory: 512Mi
      cpu: 0.5
    env:
      DATABASE_URL: { from: postgres.main.url }
      API_KEY:      { secret: true, required: true }
    replicas: 2
    strategy: rolling          # rolling | recreate | blue-green
databases:
  main:
    engine: postgres
    version: "16"
    backup: { schedule: "0 3 * * *", retain: 7 }
```

Version this from day one. Write a JSON Schema for it and validate in CI and at deploy time.

### 4.10 Secrets: envelope encryption

Per-app data encryption keys, wrapped by a master key held only by the control plane. Agents receive decrypted env at container-create time and never persist it. Use `filippo.io/age` for the crypto primitives. Master key can be sourced from file, env, or an external KMS interface added later.

### 4.11 AI layer: MCP server over the platform API

A separate binary or a subcommand. Tools: list apps, get logs, get metrics, diagnose a crashloop, trigger deploy, rollback, explain a failed build. It is a thin wrapper once the HTTP API is well designed, which is the point. Build it in a later phase, but design the HTTP API assuming it will exist. This is the concrete meaning of "AI-ready" in section 1 — the API contract, not the reconciler, carries that responsibility.

### 4.12 Frontend

React, Vite, TypeScript, Tailwind. Embedded into the Go binary as static assets. Server-sent events for live log tailing and deploy progress, not websockets, because SSE reconnects cleanly and is simpler through proxies.

Route-level code splitting from the start. The dashboard should not ship the log viewer's bundle.

The failure mode to avoid is Coolify's: dashboards that degrade as resource count grows. Design the resource list to be virtualized and paginated from day one, and never fetch the full resource graph on page load.

---

## 5. Repo layout

```
/cmd
  /levelrail          # control plane binary
  /levelrail-agent    # node agent binary
  /levelrail-mcp       # MCP server binary
/internal
  /brand             # branding indirection, see section 3
  /api               # HTTP handlers, versioned under /api/v1
  /reconcile         # controllers, one file per resource type
  /docker            # Docker Engine API wrapper, no CLI shelling
  /build             # BuildKit client, Railpack detection
  /ingress           # embedded Caddy driver
  /telemetry         # metrics + log storage and query
  /secrets           # envelope encryption
  /store             # SQLite, migrations, queries
  /spec              # app.yaml parsing + JSON Schema validation
  /agent             # agent-side logic, shared transport interface
  /network           # WireGuard mesh (phase 4)
/proto               # gRPC definitions for control plane <-> agent
/web                 # React frontend
/docs                # user-facing docs, becomes the glinr.com/levelrail pages — PUBLIC, ships with the repo
/docs-local          # PRIVATE, gitignored — competitive research, naming trail, strategy. Never referenced from /docs or source. See section 8a.
/test
  /e2e               # full stack tests against real Docker
  /fixtures          # sample apps to deploy in tests
/adr                 # architecture decision records
```

---

## 6. Phases

No timelines. Each phase has an exit criterion that is a demonstrable behavior, not a feature checklist.

### Phase 0: Research and foundation

**Exit criterion:** you can explain, in writing, exactly how Coolify handles a failed health check during a rolling deploy, and you have a running skeleton that reconciles one hardcoded container.

Work:

- Clone and read, do not fork: `coollabsio/coolify`, `Dokploy/dokploy`, `caprover/caprover`, `dokku/dokku`, `basecamp/kamal`. Kamal is the most instructive because it is the smallest and its author had strong opinions about what to leave out.
- Write `docs-local/research/prior-art.md` covering, for each: how they handle zero-downtime cutover, how they detect a bad deploy, how they roll back, how they store state, what their idle resource usage is, what their most common GitHub issues are. That last one is the most valuable signal available. Sort their issue tracker by comment count. **This lives in `docs-local`, not `/docs` — it's competitive intelligence, not user-facing documentation, and it must never ship in the public repo (see section 8a).**
- Read the BuildKit Go client examples and get a programmatic build working standalone.
- Read Caddy's library embedding docs and get a programmatic reverse proxy with auto TLS working standalone.
- Scaffold the repo, CI, linting, the brand indirection layer, and the SQLite store with migrations.
- Write ADR 001 through 008 capturing the decisions in section 4, with the rejected alternatives and why.
- Build the reconcile loop skeleton with one controller that keeps a single hardcoded nginx container running. Kill the container by hand, watch it come back. That is the whole phase 0 demo.

### Phase 1: Single node, deploy something real

**Exit criterion:** thesvg.org deploys from a git push, serves over HTTPS with a valid cert, and you can roll back to the previous version in one click.

Work:

- Docker Engine API wrapper with event stream consumption.
- Application controller: desired state to running containers, with readiness and liveness probes.
- BuildKit integration: Dockerfile builds with remote cache, build log streaming over SSE.
- Git integration: GitHub App or deploy keys, webhook receiver, commit SHA tracking.
- Embedded Caddy ingress with automatic TLS and domain routing.
- Rolling deploy: start new container, wait for readiness, cut traffic, drain and stop old. Blue-green as the default because it is easier to get right than rolling with a single replica.
- Rollback: keep the previous N images pinned so garbage collection cannot orphan a rollback target. This is a real bug in competitors.
- Secrets: envelope encryption, env injection at create time.
- Managed Postgres and Redis as first-class resources with volume management.
- Frontend: app list, app detail, deploy history, live build logs, env editor, domain config.
- Single admin user, session auth. No teams, no RBAC yet.
- `app.yaml` parsing and validation.

Deliberately excluded from Phase 1: multi-server, templates, backups to S3, teams, notifications, Compose support.

### Phase 2: Observability, the actual differentiator

**Exit criterion:** you can answer "why was this app slow at 3am last Tuesday" without leaving the dashboard and without having installed anything extra.

Work:

- Node-local metrics store, container stats collection at 15s resolution.
- Node-local log store with full text search, structured log parsing where the app emits JSON.
- Query API with time range, filtering, and aggregation. Federated across agents even though there is only one node right now.
- Frontend: per-app metrics dashboard, log viewer with search and live tail, deploy markers overlaid on metric charts. That overlay is the feature that makes the product feel different, because it makes "which deploy caused this" a visual question instead of an investigation.
- Alerting: threshold rules, notification via webhook, email, Slack, Discord, Telegram.
- Prometheus remote read endpoint.
- Crashloop detection with the last 200 lines of the failing container's logs surfaced automatically in the UI. Nobody does this well and it is the single most common thing a user needs when a deploy fails.

### Phase 3: Multi-node

**Exit criterion:** you add a second server from the UI, move an app to it, and the app's internal database connection keeps working across machines.

Work:

- Agent enrollment flow with one-time join tokens and certificate issuance.
- Real gRPC transport replacing the in-process one, with reconnection, backpressure, and version negotiation.
- WireGuard mesh setup and peer distribution.
- Internal DNS resolving service names across nodes.
- Placement: manual node assignment first. Simple spread scheduling second. No bin-packing.
- Dedicated build nodes so builds do not compete with production workloads for CPU.
- Distributed cert storage in the database, shared across ingress instances.
- Node health, drain, and cordon.

### Phase 4: Platform surface

**Exit criterion:** someone who is not you installs it, deploys their app, and does not need to ask you a question.

Work:

- Teams, projects, environments, RBAC with a small role set. Resist the urge to build a permission matrix.
- Docker Compose support as a deploy target.
- A curated template catalog. Ten to twenty services, each actually tested, each with sane defaults and a documented upgrade path. Quality over count.
- Backups: database dumps and volume snapshots to S3 compatible storage, with a restore path that has been tested by actually restoring.
- Preview environments per pull request.
- CLI: `levelrail deploy`, `levelrail logs`, `levelrail exec`, `levelrail rollback`.
- MCP server.
- Integration of first-party GLINCKER libraries: theauth for the auth layer, thesvg for the icon system. Do this here, not earlier, so the platform's own auth is boring and proven first.
- One-command install script, and a documented upgrade path that has been tested from every prior release.

### Phase 5: Hardening and public release

**Exit criterion:** you have run it in shadow next to your existing Coolify setup for long enough to see it survive things you did not plan for.

Work:

- Chaos testing: kill the Docker daemon mid-deploy, fill the disk, sever the agent connection, corrupt an image, OOM a container during readiness check. Each one gets a test.
- Upgrade and downgrade testing across versions with real data.
- Security review: container escape surface, secret handling, agent certificate validation, the install script itself.
- Performance: 100 apps on one control plane, measure idle CPU and memory, publish the number. That number is your marketing.
- Docs site at glinr.com/levelrail, including an honest comparison page and a migration guide from Coolify and Dokploy.
- Migrate thesvg.org and the other GLINCKER apps as the reference deployment, and say so publicly.
- License decision. Apache 2.0 or AGPL. AGPL protects against a cloud provider hosting it, Apache maximizes adoption. Decide before the first public commit, because changing it later requires contributor consent.

---

## 7. Engineering standards

These are the rules agents must follow.

**Testing**

- Table-driven tests for all pure logic.
- Reconcilers get tests against a fake Docker client that can be told to fail in specific ways. Every reconciler must have a test for the case where the operation half-succeeded.
- E2E tests run against real Docker in CI, deploying real fixture apps.
- Coverage gate at 70% for `/internal`, enforced in CI. Do not chase 100%, it produces tests that assert implementation details.
- Changed-file coverage check on PRs, so new code cannot lower the bar even if overall coverage passes.

**Code quality**

- `golangci-lint` with a strict config: errcheck, govet, staticcheck, gosec, revive, ineffassign, unparam.
- No `interface{}` or `any` in exported signatures without a written reason.
- Errors wrapped with context at every layer boundary. No bare `return err` across a package boundary.
- Context propagation everywhere. Every blocking call takes a context.
- No global state except the brand config and the logger.
- Structured logging with `log/slog`. Every log line that describes a resource includes its ID.

**Frontend**

- TypeScript strict mode, no `any`.
- Route-level code splitting, enforced with a bundle size budget in CI.
- Every list that can exceed 50 items is virtualized.
- No data fetching in component bodies. TanStack Query with explicit cache keys.

**Process**

- One ADR per architectural decision, in `/adr`, numbered, with the rejected alternatives written down.
- Conventional commits.
- Every PR must state what it does not do.
- Feature flags for anything half-built. No long-lived branches.
- **No em dashes (—) or en dashes (–) anywhere** — code, comments, commit messages, docs, PR bodies. Use commas, periods, parentheses, or colons.
- No direct commits/pushes to `main`. Always branch + PR. Do not open the PR until explicitly told to.

## 8. How to run Claude Code agents on this

Parallel agents work well for research and for independent feature slices. They work badly on the reconciler core, because that code needs one coherent mental model.

**Good parallel work:**

- Research agents in Phase 0, one per competitor, each producing a section of `prior-art.md`.
- Frontend pages, one agent per route, against a frozen API contract.
- Template catalog entries in Phase 4, one agent per service.
- Test writing for existing code.
- Docs.

**Do not parallelize:**

- The reconcile loop and its controllers. One agent, one long session, human review of every decision.
- The agent transport protocol.
- The database schema.
- Anything touching secrets.

**Session hygiene:**

- Every session starts by reading this file and the relevant ADRs.
- Give each agent an explicit exit criterion and a list of files it may touch.
- Require a plan before code on anything above trivial. Review the plan.
- After each merged feature: run the full test suite, run the linter, check the coverage delta, and update this file if a decision changed.

### 8a. docs-local convention (this repo specifically)

`docs-local/` is real, permanent, and lives on disk exactly like any other directory here — it's just excluded from git via `.gitignore`. Nothing here ever gets committed, pushed, or referenced from `/docs`, source, or commit messages.

What goes here:

- `docs-local/research/` — competitor prior-art analysis (`prior-art-<name>.md` per competitor, `prior-art.md` as the synthesized index), pattern extraction, anything that guides a design decision but would be awkward or competitively unwise to publish verbatim.
- `docs-local/research/naming/` — the full four-round naming trail (65 candidates, registry verification results, the reasoning that landed on Levelrail).
- `docs-local/competitor-clones/` — shallow clones of Coolify, Dokploy, CapRover, Dokku, Kamal, kept as read-only reference for agents doing research. Never modified, never a source dependency, never fetched/vendored into `/internal` or `/web`.
- Anything else that's strategy, launch planning, or pre-decision material.

Before every commit: `git status` and eyeball it. `docs-local/` should never appear staged. If it ever does, that's a stop-and-check moment, not a force-through.

## 9. Open decisions that need your input

1. License: Apache 2.0 or AGPL. This has to be settled before the first public commit.
2. Whether the control plane and agent ship as one binary with a mode flag, or two binaries. One binary is simpler to distribute, two is cleaner.
3. Whether to support rootless Docker or Podman as a runtime target. Adds surface, opens a security-conscious audience.
4. How aggressive to be about the metrics retention default, since it trades disk against usefulness.
5. Whether the first public release is announced at all, or quietly used for GLINCKER apps for a few months first. Quiet is probably right. The first impression in this category is hard to reset.
6. When to create the actual GitHub repo under GLINCKER and push. Everything up to that point stays local-only.

## 10. The main risk

It is not competition and it is not code volume. It is that a deploy platform's reputation is set by its worst failure, and the worst failures are ones you cannot find by writing more code. They surface when a health check passes on a container that had already OOMed, or when a cert renewal fails silently at 3am, or when a rollback target got garbage collected.

The mitigation is Phase 5 and the shadow-running period, and taking them seriously rather than treating them as a formality before launch. Run it alongside the existing setup with real traffic for long enough that boring problems have had a chance to happen.
