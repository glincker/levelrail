# Levelrail

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

**[Read the docs at levelrail.com](https://levelrail.com)**

Levelrail is a self-hosted, open-source deployment platform: an alternative to Heroku, Vercel, and Railway that runs on your own Linux servers. Push to a git repo and get a running app with HTTPS, logs, metrics, and one-click rollback.

It is built for people running 3 to 50 services on 1 to 10 machines who do not want to learn Kubernetes. Each server runs a small agent that talks to Docker's Engine API directly, so there is no SSH and no shelling out to the `docker` CLI. Metrics and logs are part of the core, not a Grafana you install afterwards.

<p align="center">
  <img src="docs/assets/screenshots/app-overview.png" alt="Levelrail app overview: live metrics and deploy history in one view" width="900">
</p>

## Install

You need a Linux server (`amd64` or `arm64`) with systemd, root access, and ports 80 and 443 open. Docker is installed for you if it is missing.

```
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

The installer checks the host, starts Levelrail as a systemd service, and prints a dashboard URL with a one-time setup token. Open it, choose a password, and the setup wizard takes you through a domain, a git provider, and your first app. About ten minutes, start to finish.

Prefer containers?

```
docker run -d -p 127.0.0.1:8080:8080 -v /var/run/docker.sock:/var/run/docker.sock \
  -v levelrail-data:/var/lib/levelrail-data ghcr.io/glincker/levelrail:beta
```

Images for `linux/amd64` and `linux/arm64` are published as `:beta` (use this until a stable release ships) and `:edge`. Upgrade and uninstall commands, pinning a version, the compose file, and the node agent image are in [Installing](docs/installing.md) and [Docker](docs/docker.md). Walkthrough: [Getting started](docs/getting-started.md).

## What you get

- **Safe deploys.** Rolling, recreate, or blue-green, gated on real readiness probes. Prior images are pinned, so rollback is always one click and garbage collection cannot remove the target.
- **Observability built in.** Node-local metrics at 15 second resolution and full-text log search, with deploy markers drawn on the charts so "which deploy caused this" is visible at a glance.
- **Databases that verify their own backups.** Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB, Dragonfly, and ClickHouse, with scheduled backups, restore, and an automatic re-download and checksum after every backup.
- **Many servers, no open ports.** Add nodes with a one-time token. Agents dial out over mTLS, with WireGuard mesh networking, internal DNS, cordon, and drain.
- **Git-native.** GitHub, GitLab, Bitbucket, and Gitea webhooks, preview environments per pull request, and an import box that accepts a repo URL, an image, a `docker run` command, or a compose file.
- **301 one-click templates** for self-hosted services, each a compose file the platform deploys and manages.
- **Access control and audit.** Allow and Deny IAM policies scoped to a single app or database, with a full audit log and CSV export, in the free Apache 2.0 core.
- **Alerting.** Threshold, crashloop, and certificate expiry rules delivered over 18 notification kinds, including Slack, Discord, email, Telegram, PagerDuty, ntfy, and a generic webhook.
- **AI-ready.** The HTTP API the dashboard uses also backs an MCP server with 155 tools, so an AI assistant can list apps, read logs, and diagnose a crashloop. AI never sits in the reconciliation path.

<table>
  <tr>
    <td width="50%">
      <img src="docs/assets/screenshots/apps-list.png" alt="Levelrail apps list showing all services across nodes at a glance" width="420"><br>
      <sub>Apps list</sub>
    </td>
    <td width="50%">
      <img src="docs/assets/screenshots/deploy-history.png" alt="Levelrail deploy history view with one-click rollback" width="420"><br>
      <sub>Deploy history and rollback</sub>
    </td>
  </tr>
  <tr>
    <td width="50%">
      <img src="docs/assets/screenshots/logs.png" alt="Levelrail live log viewer with full-text search" width="420"><br>
      <sub>Live log viewer</sub>
    </td>
    <td width="50%">
      <img src="docs/assets/screenshots/nodes.png" alt="Levelrail nodes list showing node health and placement" width="420"><br>
      <sub>Nodes</sub>
    </td>
  </tr>
</table>

## Status: pre-release

Levelrail is in active development and has no stable release yet. APIs, the app spec format, and the on-disk layout can change between betas, so do not run production workloads on it yet.

Maturity is uneven, and we label it feature by feature in [Feature status](docs/feature-status.md). Single node is the well-tested path. Deploy approvals, database backups with point-in-time recovery, and GitHub previews are marked stable, each with a live end-to-end test. Multi-node enrollment, drain, and placement have run against two real Docker daemons, but the WireGuard mesh across hosts is still unverified. Most other areas are beta, and a few (in-app AI chat, GPU models, the load balancer, platform as code, Cloudflare tunnel) are off by default behind [`APP_EXPERIMENTAL`](docs/experimental-features.md). The [roadmap](docs/roadmap.md) has the current list of what is done and what is next.

## Why not Coolify or Dokploy?

Most self-hosted platforms in this category manage servers by SSHing in and shelling out to the `docker` CLI, then parsing text. That is a common source of flakiness, and it forces polling loops. Levelrail does it differently:

- **Agent-based.** A small agent on each node uses the Docker Engine API directly and streams container events up. The control plane never polls.
- **Observability in the core.** Metrics and logs ship with the platform.
- **Small footprint.** One static Go binary for the control plane and one for the agent, with embedded SQLite, an embedded Caddy ingress, and an embedded dashboard. Measured numbers and conditions are in [Performance](docs/performance.md).

| | Levelrail | Coolify | Dokploy | CapRover | Dokku | Kamal |
| --- | --- | --- | --- | --- | --- | --- |
| Node control | Reverse-dialed agent | SSH + CLI | SSH + Engine API | Swarm API | Local bash | SSH, one-shot |
| Orchestration | Level-triggered reconciler | Compose | Swarm | Swarm | Bash scheduler | None |
| Built-in metrics and logs | Yes | Opt-in add-on | Separate binary | No | No | No |
| Rollback | Pinned images, readiness-gated | Health check off by default | Swarm rollback | Manual | Default scheduler: none | Proxy health gate |
| Backup verification | Automatic, all 8 engines | Non-empty check | None | None | No backups | No backups |

This is positioning, not a ranking. All of them are worth using. The one area where Levelrail trails is breadth of templates: 301 curated entries against Coolify's roughly 370, an intentional bet on curation (see [ADR 015](adr/015-service-template-catalog-reversal.md)). The full, sourced comparison is in [Comparison](docs/comparison.md).

## How it is built

- **Control plane** (`cmd/levelrail`): one Go binary with the dashboard embedded. It reconciles declarative desired state against observed Docker state, the Kubernetes controller pattern without the rest of Kubernetes.
- **Node agent** (`cmd/levelrail-agent`): dials out to the control plane. In single-node mode it runs in-process, so one node and ten nodes share one code path.
- **Ingress:** [Caddy](https://caddyserver.com/) embedded as a library, for automatic TLS and routing.
- **Builds:** [BuildKit](https://github.com/moby/buildkit) as a library, with Railpack detection for apps without a Dockerfile.
- **State:** SQLite in WAL mode via `modernc.org/sqlite`, pure Go.
- **CLI and MCP:** `levelrail-cli` and `levelrail-mcp` are thin clients over the same HTTP API under `/api/v1`.

Details: [Architecture](docs/architecture.md).

## Build from source

Needs Go 1.26 or newer and Docker.

```
go build ./cmd/levelrail
go build ./cmd/levelrail-agent
go build ./cmd/levelrail-cli
```

The frontend is a Vite project in `web/` that is embedded into the control plane at build time. See [Installing](docs/installing.md#option-3-build-from-source) and [CONTRIBUTING.md](CONTRIBUTING.md) for the dev loop, tests, and commit conventions.

## Docs and community

- [levelrail.com](https://levelrail.com) -- the hosted docs site: getting started, architecture, app spec reference, roadmap, full index
- [docs/](docs/README.md) -- the same content as plain Markdown, for browsing directly on GitHub
- [GitHub Discussions](https://github.com/glincker/levelrail/discussions) -- questions, ideas, show and tell
- [GLINR Discord](https://discord.gg/Ar5pcaZB99) -- live chat with maintainers and other users, with a dedicated `#levelrail` forum channel for questions and support
- [support@levelrail.com](mailto:support@levelrail.com) -- direct email support

<a href="https://discord.gg/Ar5pcaZB99"><img src="https://discord.com/api/guilds/829168897080557579/widget.png?style=banner2" alt="Join the GLINR Discord" /></a>

## Star history

[![Star History Chart](https://api.star-history.com/svg?repos=glincker/levelrail&type=Date)](https://star-history.com/#glincker/levelrail&Date)

## License

Apache 2.0, see [LICENSE](LICENSE).
