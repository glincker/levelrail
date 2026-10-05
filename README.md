<div align="center">

<h1>Levelrail</h1>

<p><strong>Push to git, get a running app with TLS, logs, metrics, and rollback, on your own Linux boxes.</strong></p>

<img src="docs/assets/brand/typing.svg" alt="Push to git. Get a running app. TLS, logs, metrics, and rollback built in. No SSH, no Grafana, no Kubernetes. Self-hosted on your own Linux boxes." width="560" height="40">

<p>
  <a href="https://levelrail.com">Docs</a> ·
  <a href="#quickstart">Quickstart</a> ·
  <a href="#features">Features</a> ·
  <a href="#a-tour-of-the-dashboard">Screenshots</a> ·
  <a href="docs/comparison.md">Compare</a> ·
  <a href="https://discord.gg/Ar5pcaZB99">Discord</a>
</p>

<p>
  [![CI](https://github.com/glincker/levelrail/actions/workflows/ci.yml/badge.svg)](https://github.com/glincker/levelrail/actions/workflows/ci.yml)
  [![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
  [![Go Report Card](https://goreportcard.com/badge/github.com/glincker/levelrail)](https://goreportcard.com/report/github.com/glincker/levelrail)
  [![Go Version](https://img.shields.io/github/go-mod/go-version/glincker/levelrail)](go.mod)
  [![GitHub stars](https://img.shields.io/github/stars/glincker/levelrail?style=flat)](https://github.com/glincker/levelrail/stargazers)
  [![Last commit](https://img.shields.io/github/last-commit/glincker/levelrail)](https://github.com/glincker/levelrail/commits/main)
  [![PRs welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
  [![Discussions](https://img.shields.io/github/discussions/glincker/levelrail)](https://github.com/glincker/levelrail/discussions)
  [![Discord](https://img.shields.io/discord/829168897080557579?style=flat-square&logo=discord&logoColor=white&label=discord&color=5865F2)](https://discord.gg/Ar5pcaZB99)
  [![Status: pre-release](https://img.shields.io/badge/status-pre--release-orange.svg)](#status)
  [![Docs](https://img.shields.io/badge/docs-levelrail.com-f59e0b.svg)](https://levelrail.com)
</p>

<img src="docs/assets/screenshots/app-overview.png" alt="Levelrail app overview: live metrics and deploy history in one view" width="900">

</div>

<details>
<summary><b>Table of contents</b></summary>

- [What is Levelrail?](#what-is-levelrail)
- [Quickstart](#quickstart)
- [Features](#features)
- [Status](#status)
- [Why not Coolify or Dokploy](#why-not-coolify-or-dokploy-self-hosted-herokuvercel-alternative)
- [Architecture at a glance](#architecture-at-a-glance)
- [How it compares](#how-it-compares)
- [A tour of the dashboard](#a-tour-of-the-dashboard)
- [Building and running locally](#building-and-running-locally)
- [Contributing](#contributing)
- [Docs and community](#docs-and-community)
- [Founding partners](#founding-partners)
- [Meet the founder](#meet-the-founder)
- [License](#license)

</details>

## What is Levelrail?

Levelrail is a self-hosted, open-source PaaS: an alternative to Heroku,
Vercel, and Railway for teams who would rather run their own deployment
platform than rent one. Point it at one or more Linux boxes and it
turns them into a private cloud. Built for 3-50 services across 1-10
machines, not a Kubernetes competitor.

- **No SSH, no shelling out.** An agent on each node talks to Docker's
  Engine API directly, so there are no polling loops and no parsed `docker`
  output.
- **Observability is part of the core.** Metrics and log search ship in the
  binary, with deploy markers drawn on the charts. No Grafana install.
- **Small by design.** One Go binary for the control plane, one for the
  agent, SQLite for state.
- **AI-ready, not AI-driven.** The HTTP API that runs the dashboard also backs
  an MCP server. AI reads and suggests; it is never in the reconcile path.

If this solves a problem you have, a star helps other people building
the same thing find it.

## Quickstart

Needs a Linux server (`amd64` or `arm64`) with systemd, root access,
and ports 80/443/8080 free. Docker is installed for you if it's
missing. Pick whichever fits how you run things. Full details, env var
overrides, verifying the install, upgrading, and uninstalling are all
in [docs/installing.md](docs/installing.md).

### Your first five minutes

<table>
  <tr>
    <td width="33%" valign="top">
      <a href="docs/installing.md#first-sign-in"><img src="docs/assets/screenshots/setup-wizard.png" alt="Levelrail setup wizard running server checks on a fresh install"></a><br>
      <b>1. Install and sign in</b><br>
      <sub>One command, then the setup wizard checks the server and walks you to a first app.</sub>
    </td>
    <td width="33%" valign="top">
      <a href="docs/git-integrations.md"><img src="docs/assets/screenshots/app-source.png" alt="Levelrail Git source settings with GitHub, GitLab, Bitbucket, Gitea, and URL tabs"></a><br>
      <b>2. Connect a repo</b><br>
      <sub>Pick a provider, let auto-detect choose the build, deploy on every push.</sub>
    </td>
    <td width="33%" valign="top">
      <a href="docs/deployments-page.md"><img src="docs/assets/screenshots/deployments-all.png" alt="Levelrail Deployments page listing recent deploys and a rollback"></a><br>
      <b>3. Watch it ship</b><br>
      <sub>Every deploy is listed live, and rolling back is one click.</sub>
    </td>
  </tr>
</table>

**Linux server, recommended:**

```
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

Checks the host first, installs Docker if it's missing, sets up a
systemd unit, waits for the control plane to report healthy, then
prints a dashboard URL and a one-time setup token for creating the
first admin. Open that URL, paste the token (it's pre-filled if you
click the printed link), pick a password, and the setup wizard walks
you through a domain, git provider, and your first app. `sh -s upgrade`
and `sh -s uninstall` do what they say.

**Already running everything as containers:**

```
docker run -d -p 127.0.0.1:8080:8080 -v /var/run/docker.sock:/var/run/docker.sock \
  -v levelrail-data:/var/lib/levelrail-data ghcr.io/glincker/levelrail:beta
```

Or with the [committed `docker-compose.yml`](docker-compose.yml):

```
curl -fsSLO https://raw.githubusercontent.com/glincker/levelrail/main/docker-compose.yml
export DOCKER_GID=$(getent group docker | cut -d: -f3)
docker compose up -d
```

`ghcr.io/glincker/levelrail` and `ghcr.io/glincker/levelrail-agent` are
published for `linux/amd64` and `linux/arm64` on every tagged release
under `:beta` (use this until a stable release ships), `:edge`, and,
once one ships, `:latest`; see [docs/docker.md](docs/docker.md) for the
full `docker run` and compose examples, including the node agent.

**Building from source:**

```
go build ./cmd/levelrail
```

See [docs/getting-started.md](docs/getting-started.md) for
requirements and running it locally.

See [docs/comparison.md](docs/comparison.md) for how this differs from
Coolify, Dokploy, CapRover, Dokku, and Kamal, including what Levelrail
doesn't do yet.

## Features

- **Zero-downtime deploys.** Rolling, recreate, or blue-green strategy,
  gated on real readiness/liveness probes, with rollback to pinned
  prior images always available, not something to reconstruct by hand.
- **Observability built in.** Node-local metrics at 15s resolution and
  full-text log search, no separate Grafana/Loki install. Deploy
  markers are overlaid directly on metric charts, so "which deploy
  caused this" is a visual answer, not an investigation.
- **Eight managed database engines.** Postgres, Redis, MySQL, MongoDB,
  MariaDB, KeyDB, Dragonfly, and ClickHouse, all through one engine
  registry: scheduled backups, restore, and automatic post-backup
  verification apply generically across every engine.
- **Multi-node from day one.** WireGuard mesh, internal DNS across
  nodes, cordon/drain, and no inbound ports required on any managed
  server.
- **Git-native deploys.** GitHub, GitLab, and Bitbucket webhooks, plus
  preview environments per pull request with automatic teardown.
- **IAM and audit.** AWS-IAM-shaped Allow/Deny policies scoped to a
  specific resource, with a full audit log and CSV export.
- **Alerting across seventeen channels.** Threshold, crashloop, certificate
  expiry, and five other rule kinds, delivered to Slack, Discord, email,
  Telegram, Pushover, PagerDuty, Microsoft Teams, Resend, Gotify, Ntfy,
  Mattermost, Lark, Rocket.Chat, Opsgenie, Webex, Google Chat, or a webhook.
- **AI-ready API.** The same HTTP API the dashboard runs on backs an
  MCP server, so AI tools can list apps, read logs, and diagnose a
  crashloop directly.

## Status

Early, active development. Single node is the well-tested path. Multi-node
runs too: agent enrollment, cordon, and drain with real container
relocation have now been verified against two real Docker daemons,
alongside internal DNS and node placement (see
[docs/multi-node-quickstart.md](docs/multi-node-quickstart.md)); the
WireGuard mesh itself and cross-host remote transport are still
unverified. Cloud node provisioning now covers five providers (Hetzner,
DigitalOcean, AWS, Azure, GCP), and apps can redeploy a branch's latest
commit on a cron schedule. Beyond the core deploy path, an IAM-style
policy engine, audit logging, feature flags, alerting across
multiple rule kinds and eighteen notification channels, a self-service
team invite flow, and eight managed database engines with
backup/restore/verification are also shipped
(see [docs/roadmap.md](docs/roadmap.md) for the full, current list).
Day-to-day operation is covered too: a Status page and `attention` CLI
command that list everything needing action, a disk pressure banner,
certificate expiry countdowns and stalled-renewal detection, node
connection history, and a log viewer with level filters and expandable
rows.
There is no stable release yet: the only published image tag is `:beta`,
and the project is not ready for production workloads. APIs, the app
spec format, and the on-disk data layout can all still change without
notice.

Maturity is uneven. Per [docs/feature-status.md](docs/feature-status.md),
only deploy approvals, database backups with point-in-time recovery, and
GitHub previews are labeled stable (each has a live end-to-end test).
Most other areas, including IAM, multi-node and WireGuard, the 17
notification channels, and the MCP server, are beta, and the in-app AI
chat, GPU models, load balancer, platform as code, and Cloudflare tunnel
are hidden behind flags, off by default via
[docs/experimental-features.md](docs/experimental-features.md)'s
`APP_EXPERIMENTAL` switch. Multi-node's join flow is the one area with a
documented real-infrastructure run, across two real Docker daemons
rather than a fresh VPS; see
[docs/multi-node-quickstart.md](docs/multi-node-quickstart.md) for what
that verification did and did not cover. No feature has been verified on
a fresh VPS with a real public domain yet. The full per-feature list is
in [docs/feature-status.md](docs/feature-status.md).

From the team behind [thesvg](https://github.com/glincker/thesvg) (6,400+ brand SVG icons) and [theauth-go](https://github.com/glincker/theauth-go) (OAuth 2.1 auth library for Go).

## Why not Coolify or Dokploy (self-hosted Heroku/Vercel alternative)

Most self-hosted platforms in this category drive remote servers by
SSHing in and shelling out to the `docker` CLI, then parsing its text
output. That's a common source of flakiness and it forces polling loops
to detect state changes. Levelrail takes a different approach:

- **Agent-based control plane.** A small agent runs on each node, talks
  to the local Docker Engine API directly, and streams container events
  up to the control plane, which never polls.
- **Observability built in, not bolted on.** Metrics and log storage are
  first-class parts of the core, not something you're told to install
  separately.
- **Low idle footprint.** A single static Go binary for the control
  plane, a single static Go binary for the agent, no separate database
  server, no message queue, no extra containers just to run the
  platform itself. Measured numbers and conditions (currently a macOS
  dev build; no Linux release-build number yet) are in
  [docs/performance.md](docs/performance.md).

Levelrail is not a Kubernetes competitor. It targets teams running
somewhere between 3 and 50 services across 1 to 10 machines who want a
private cloud without taking on Kubernetes's operational surface.

## Architecture at a glance

- **Control plane** (`cmd/levelrail`): a single Go binary. Reconciles
  declarative resource records against observed Docker state, the same
  pattern Kubernetes controllers use, without the rest of the Kubernetes
  runtime.
- **Node agent** (`cmd/levelrail-agent`): dials out to the control plane
  (no inbound ports on managed servers) and talks to the local Docker
  socket via the Engine API.
- **Ingress**: [Caddy](https://caddyserver.com/) embedded as a Go
  library (`internal/ingress`), driven in-process, for automatic TLS and
  domain routing.
- **Builds**: [BuildKit](https://github.com/moby/buildkit) as a Go
  library (`internal/build`), not `docker build`, for remote cache and
  parallel stage execution.
- **State**: embedded SQLite in WAL mode (`internal/store`), pure Go via
  `modernc.org/sqlite`, so cross-compiling the binary stays simple.
- **API**: an HTTP API under `internal/api`, versioned at `/api/v1`.
- **Frontend** (`web/`): React, Vite, TypeScript, Tailwind, TanStack
  Router and Query. Built as static assets and embedded into the control
  plane binary via `embed.FS`, so there's no separate Node process to
  run in production.

Everything ships as two binaries: `levelrail` (control plane, with the
frontend embedded) and `levelrail-agent` (node agent). In single-node
mode the agent's transport runs in-process instead of over the network,
so the code path is the same whether you're running one node or ten.

## How it compares

Positioning, not a ranking. All of these are worth using; the differences
below are the ones that matter for choosing between them.

Short version: Levelrail is the only one of these six that streams
events instead of polling, verifies backups instead of trusting them,
and pins rollback images so garbage collection can't eat them.

| Project | Node control | Orchestration | Observability | Ingress | Rollback |
| --- | --- | --- | --- | --- | --- |
| **Levelrail** | Reverse-dialed gRPC agent, no CLI shelling | Custom Go reconciler over Docker Engine API, level-triggered | Node-local metrics and logs, federated query, all shipped | Embedded Caddy, in-process | Prior images pinned so garbage collection can't orphan a rollback target, cutover gated on a real readiness probe |
| Coolify (v4) | SSH plus CLI-shelled `docker`/`docker compose` | Docker Compose per app, Traefik label discovery | Optional bolt-on agent, opt-in | Traefik, separate container | Health check off by default, a broken deploy can be marked successful |
| Dokploy | SSH-tunneled Docker Engine API plus CLI-shelled lifecycle ops | Docker Swarm services | Separate Go binary, polling | Traefik, Swarm service | Swarm's own RollbackConfig, marked done before health is verified |
| CapRover | Docker Swarm API, even single-node | Docker Swarm services | Optional sibling containers, not built in | nginx, sibling Swarm service | None: manual re-tag and re-deploy by hand |
| Dokku | Local `dokku` bash entrypoint, no daemon | Custom Bash scheduler over plain `docker` | Not stated in research | nginx by default, pluggable | None on the default scheduler, only on the newer, opt-in Kubernetes-backed one |
| Kamal | One-shot SSH CLI, no daemon or agent | None: deploy script, not a control plane | None built in | `kamal-proxy`, standalone container | `kamal-proxy`'s own two-stage health gate, but no image pinning or rollback command |

### Where the feature depth shows

A handful of concrete points, sourced from actually reading each
competitor's own code, not cherry-picked framing. Full detail with
per-line citations: [docs/comparison.md](docs/comparison.md)'s feature
matrix.

- **Backup verification.** Levelrail re-downloads, re-hashes, and
  compares every backup against what was recorded at backup time,
  across all 8 managed database engines. None of the other five
  projects researched here run any automated check on a backup after
  taking it: Coolify checks only that the dump file is non-empty,
  Dokploy and CapRover do no check at all, and Dokku and Kamal have no
  built-in backup feature in the first place.
- **AI-agent surface.** MCP tools covering apps, deploys, databases,
  nodes, domains, and more (`cmd/levelrail-mcp`), with a small
  `agent-core` profile for constrained context budgets. Of the other
  projects researched here, only Coolify ships an MCP server. The MCP
  server is beta, see [docs/feature-status.md](docs/feature-status.md);
  current tool counts by toolset are generated and kept current at
  [docs/mcp-tool-surface.md](docs/mcp-tool-surface.md), rather than a
  number here that can drift.
- **Notification channels.** 17 kinds against Dokploy's 12, the next
  closest, unit-tested against mock endpoints; none has a recorded run
  against a real vendor yet.
- **Fine-grained RBAC.** Resource-scoped IAM policies (`app:name`,
  `database:name`, or `*`) ship in the free, Apache 2.0 core. Dokploy's
  comparable granularity sits behind a paid enterprise license.

One real gap, stated plainly: the template catalog is 206 curated
entries against Coolify's 371 (an intentional curation-over-count bet,
see [ADR 015](adr/015-service-template-catalog-reversal.md)), the one
row in that matrix this project doesn't lead.

## A tour of the dashboard

Every image below is captured from a real, running control plane by
[`scripts/screenshots/capture.sh`](scripts/screenshots/capture.sh), with real
containers, metrics, and logs. Click one to open the guide for that screen.

### Deploy and observe

<table>
  <tr>
    <td width="50%" valign="top">
      <a href="docs/managing-apps-at-scale.md"><img src="docs/assets/screenshots/apps-list.png" alt="Levelrail apps list showing all services at a glance"></a><br>
      <b>Apps</b><br>
      <sub>Every service, its status, and where it runs.</sub>
    </td>
    <td width="50%" valign="top">
      <a href="docs/deploy-safety.md"><img src="docs/assets/screenshots/deploy-history.png" alt="Levelrail deploy history with one-click rollback"></a><br>
      <b>Deploy history and rollback</b><br>
      <sub>Each release is pinned, so rollback is never a rebuild.</sub>
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <a href="docs/deploying-apps.md"><img src="docs/assets/screenshots/app-deploy-settings.png" alt="Levelrail deploy settings showing blue-green strategy and replicas"></a><br>
      <b>Deploy strategy</b><br>
      <sub>Blue-green, rolling, or recreate, plus pre and post deploy hooks.</sub>
    </td>
    <td width="50%" valign="top">
      <a href="docs/observability.md"><img src="docs/assets/screenshots/app-overview-page.png" alt="Levelrail app overview page"></a><br>
      <b>App overview</b><br>
      <sub>Health, replicas, and recent activity for one app.</sub>
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <a href="docs/observability.md"><img src="docs/assets/screenshots/logs.png" alt="Levelrail live log viewer with full-text search"></a><br>
      <b>Live logs</b><br>
      <sub>Full-text search and live tail, stored on the node.</sub>
    </td>
    <td width="50%" valign="top">
      <a href="docs/cost-estimate.md"><img src="docs/assets/screenshots/app-resources.png" alt="Levelrail resource suggestion and cost estimate for an app"></a><br>
      <b>Right-sizing and cost</b><br>
      <sub>Limits suggested from real usage, with a what-it-costs-elsewhere estimate.</sub>
    </td>
  </tr>
</table>

### Data and edge

<table>
  <tr>
    <td width="50%" valign="top">
      <a href="docs/managing-databases.md"><img src="docs/assets/screenshots/databases-list.png" alt="Levelrail databases list"></a><br>
      <b>Managed databases</b><br>
      <sub>Postgres, Redis, MySQL, and more, one registry.</sub>
    </td>
    <td width="50%" valign="top">
      <a href="docs/managing-databases.md"><img src="docs/assets/screenshots/database-overview.png" alt="Levelrail database overview with TLS and public access settings"></a><br>
      <b>Database overview</b><br>
      <sub>TLS, host-port exposure, and backups in one place.</sub>
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <a href="docs/load-balancing.md"><img src="docs/assets/screenshots/load-balancer.png" alt="Levelrail load balancer view with upstream health"></a><br>
      <b>Load balancer</b><br>
      <sub>Per-upstream health once the ingress reconciler observes it.</sub>
    </td>
    <td width="50%" valign="top">
      <a href="docs/domains-and-ingress.md"><img src="docs/assets/screenshots/domains-list.png" alt="Levelrail Domains page with platform ingress settings"></a><br>
      <b>Domains and TLS</b><br>
      <sub>Primary domain, ACME certificates, and HSTS.</sub>
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <a href="docs/deploying-apps.md"><img src="docs/assets/screenshots/app-environment.png" alt="Levelrail environment variables and write-only secrets"></a><br>
      <b>Environment and secrets</b><br>
      <sub>Secrets are write-only and show their age.</sub>
    </td>
    <td width="50%" valign="top">
      <a href="docs/backups-and-storage.md"><img src="docs/assets/screenshots/backups.png" alt="Levelrail Backups page prompting for a backup target"></a><br>
      <b>Backups</b><br>
      <sub>Connect an S3-compatible target, then schedule from any database.</sub>
    </td>
  </tr>
</table>

<details>
<summary><b>Nodes, networking, and CI</b></summary>
<br>

<table>
  <tr>
    <td width="50%" valign="top">
      <a href="docs/multi-node.md"><img src="docs/assets/screenshots/nodes.png" alt="Levelrail nodes list showing health and placement"></a><br>
      <b>Nodes</b><br>
      <sub>Node health and placement across machines.</sub>
    </td>
    <td width="50%" valign="top">
      <a href="docs/network-topology.md"><img src="docs/assets/screenshots/network-topology.png" alt="Levelrail network topology graph"></a><br>
      <b>Network topology</b><br>
      <sub>How apps and nodes connect.</sub>
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <a href="docs/node-provisioning.md"><img src="docs/assets/screenshots/node-providers.png" alt="Levelrail cloud node providers settings"></a><br>
      <b>Cloud node providers</b><br>
      <sub>Create servers at Hetzner, DigitalOcean, AWS, or Azure from the Nodes page.</sub>
    </td>
    <td width="50%" valign="top">
      <a href="docs/pipelines.md"><img src="docs/assets/screenshots/pipelines.png" alt="Levelrail Pipelines first-run view with a sample pipeline file"></a><br>
      <b>Pipelines</b><br>
      <sub>CI/CD runs on your own nodes, defined in a YAML file.</sub>
    </td>
  </tr>
</table>

</details>

<details>
<summary><b>Getting started and operations</b></summary>
<br>

<table>
  <tr>
    <td width="50%" valign="top">
      <a href="docs/template-catalog.md"><img src="docs/assets/screenshots/templates-catalog.png" alt="Levelrail service templates catalog with categories"></a><br>
      <b>Template catalog</b><br>
      <sub>One-click services from a searchable catalog.</sub>
    </td>
    <td width="50%" valign="top">
      <a href="docs/installing.md#installing-just-the-cli"><img src="docs/assets/screenshots/cli-access.png" alt="Levelrail CLI access page with install and login steps"></a><br>
      <b>CLI access</b><br>
      <sub>Install the CLI and log in with a device code, no SSH key.</sub>
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <a href="docs/getting-started.md"><img src="docs/assets/screenshots/dashboard-home.png" alt="Levelrail dashboard home with stats and server checks"></a><br>
      <b>Dashboard home</b><br>
      <sub>Stats, recent activity, and a Needs attention list from the server checks.</sub>
    </td>
    <td width="50%" valign="top">
      <a href="docs/status-page.md"><img src="docs/assets/screenshots/status-page-settings.png" alt="Levelrail status page settings, switched off by default"></a><br>
      <b>Public status page</b><br>
      <sub>A read-only page with component status, off until you publish it.</sub>
    </td>
  </tr>
</table>

</details>

## Building and running locally

Requires Go 1.26+ and Docker.

```
# control plane
go build ./cmd/levelrail

# node agent
go build ./cmd/levelrail-agent
```

The frontend lives in `web/` and is a separate Vite project:

```
cd web
npm install
npm run dev       # Vite dev server
npm run build      # production build, embedded into the control plane binary
```

See `web/README.md` for frontend-specific commands and conventions.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for how to propose changes,
commit conventions, and how to run tests and the linter locally.

## Docs and community

- [levelrail.com](https://levelrail.com) -- the hosted docs site: getting started, architecture, app spec reference, roadmap, full index
- [docs/](docs/README.md) -- the same content as plain Markdown, for browsing directly on GitHub
- [Roadmap](docs/roadmap.md) -- what is shipped, in progress, and next
- [GitHub Discussions](https://github.com/glincker/levelrail/discussions) -- questions, ideas, show and tell
- [GLINR Discord](https://discord.gg/Ar5pcaZB99) -- live chat with maintainers and other users, with a dedicated `#levelrail` forum channel for questions and support
- [support@levelrail.com](mailto:support@levelrail.com) -- direct email support

<a href="https://discord.gg/Ar5pcaZB99"><img src="https://discord.com/api/guilds/829168897080557579/widget.png?style=banner2" alt="Join the GLINR Discord" /></a>

## Star history

[![Star History Chart](https://api.star-history.com/svg?repos=glincker/levelrail&type=Date)](https://star-history.com/#glincker/levelrail&Date)

## License

Apache 2.0, see [LICENSE](LICENSE).

## Founding partners

Founding partner sites and libraries, from the same studio:

| | |
| --- | --- |
| [**theSVG**](https://thesvg.org) | A searchable library of brand SVG icons for developers and designers. |
| [**theauth**](https://github.com/glincker/theauth) | Open-source auth for AI agents and humans, with MCP OAuth 2.1 and audit. |
| [**AskVerdict AI**](https://askverdict.ai) | Structured multi-agent decisions for teams, with verdicts and action items. |

## Meet the founder

Levelrail is built by Gagan Deep Singh, founder of [GLINR STUDIOS](https://glinr.com)
and [theSVG](https://thesvg.org). Say hello at [thegdsks.com](https://thegdsks.com).

<div align="center">

<br>

<a href="https://glinr.com">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/brand/glincker-light.svg">
    <img src="docs/assets/brand/glincker-dark.svg" alt="GLINR STUDIOS" width="56">
  </picture>
</a>

<sub>Powered by <a href="https://glinr.com"><b>GLINR STUDIOS</b></a> · icon from <a href="https://thesvg.org/icon/glincker">theSVG</a></sub>

</div>
