<div align="center">
<img src="docs/assets/brand/levelrail-default.svg" alt="Levelrail logo" width="56" height="56">
<h1>Levelrail</h1>
<p><strong>Self-hosted deployment platform: git push to production on your own servers and cloud accounts.</strong><br>
Git deploys, 565 one-click templates (346 proven on a real engine), edge and TLS, nodes at six clouds, managed databases with restore-drilled backups, and built-in observability. One Go binary, Apache 2.0.</p>
<p>
  <a href="https://github.com/glincker/levelrail/stargazers"><img src="https://img.shields.io/github/stars/glincker/levelrail?style=flat&logo=github" alt="GitHub stars"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/glincker/levelrail" alt="License: Apache 2.0"></a>
  <a href="https://github.com/glincker/levelrail/releases"><img src="https://img.shields.io/github/v/release/glincker/levelrail?include_prereleases&label=release" alt="Latest release"></a>
  <a href="https://github.com/glincker/levelrail/actions/workflows/ci.yml"><img src="https://github.com/glincker/levelrail/actions/workflows/ci.yml/badge.svg" alt="CI status"></a>
  <a href="https://github.com/glincker/levelrail/commits/main"><img src="https://img.shields.io/github/last-commit/glincker/levelrail?style=flat" alt="Last commit"></a>
  <a href="https://www.npmjs.com/package/levelrail-cli"><img src="https://img.shields.io/npm/v/levelrail-cli?style=flat&logo=npm&label=npm" alt="levelrail-cli on npm"></a>
  <a href="https://registry.terraform.io/providers/glincker/levelrail/latest"><img src="https://img.shields.io/github/v/release/glincker/terraform-provider-levelrail?style=flat&logo=terraform&label=terraform" alt="Terraform provider on the registry"></a>
  <a href="https://hub.docker.com/r/levelrail/levelrail"><img src="https://img.shields.io/docker/pulls/levelrail/levelrail?style=flat&logo=docker&label=docker%20pulls" alt="Docker pulls"></a>
  <a href="docs/mcp.md"><img src="https://img.shields.io/badge/MCP-server-8A2BE2?style=flat" alt="MCP server"></a>
  <a href="https://discord.gg/Ar5pcaZB99"><img src="https://img.shields.io/discord/829168897080557579?style=flat&logo=discord&logoColor=white&label=discord&color=5865F2" alt="Discord"></a>
</p>
<p>
  <a href="https://levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">Website</a> &middot;
  <a href="https://docs.levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">Docs</a> &middot;
  <a href="#install">Install</a> &middot;
  <a href="#vs-coolify-dokploy-caprover-and-dokku">Compare</a> &middot;
  <a href="#faq">FAQ</a> &middot;
  <a href="docs/roadmap.md">Roadmap</a> &middot;
  <a href="https://github.com/glincker/levelrail/discussions">Discussions</a>
</p>
<!-- i18n-nav:start -->
<p>English &middot; <a href="docs/i18n/README.zh-Hans.md">简体中文</a> &middot; <a href="docs/i18n/README.es.md">Español</a> &middot; <a href="docs/i18n/README.pt-BR.md">Português (BR)</a> &middot; <a href="docs/i18n/README.de.md">Deutsch</a> &middot; <a href="docs/i18n/README.fr.md">Français</a> &middot; <a href="docs/i18n/README.ja.md">日本語</a></p>
<!-- i18n-nav:end -->
<picture><source media="(prefers-color-scheme: dark)" srcset="docs/assets/brand/social/github-social-1280x640.jpg"><img src="docs/assets/brand/social/github-social-1280x640-light.jpg" alt="Levelrail: a self-hosted deployment platform with Git deploys, templates, edge, databases and observability." width="880"></picture>
</div>

<p align="center"><a href="docs/template-catalog.md"><b>565</b> one-click templates</a> &middot; <a href="docs/node-provisioning.md"><b>6</b> clouds</a> &middot; <a href="docs/managing-databases.md"><b>13</b> database engines</a> &middot; <a href="docs/integrations.md"><b>18</b> alert channels</a> &middot; <a href="docs/benchmarks.md"><b>~200 MB</b> idle</a> &middot; <a href="docs/architecture.md"><b>1</b> binary</a></p>

Levelrail turns Linux servers into a private cloud and works with the Kubernetes clusters and cloud accounts you already have. Pre-1.0: single-server is production ready, multi-node is beta. Every feature carries a stable, beta or experimental label in [Feature status](docs/feature-status.md), and the labels below follow it.

<p align="center"><img src="docs/assets/demos/product-tour.gif" alt="Levelrail dashboard tour: fleet overview, apps list, deploy history and built-in metrics" width="880"></p>

## What you get

Every label below links to its docs page. Maturity follows [Feature status](docs/feature-status.md).

**[Ship with git push](docs/deploying-apps.md)**
- [Git push deploys](docs/git-push-deploy.md) from GitHub, GitLab, Bitbucket
- [Preview environments per pull request](docs/deploy-previews.md)
- [Readiness-gated rollouts](docs/zero-downtime-deploys.md) and [one-step rollback](docs/rollback.md)
- [Canaries and split tests](docs/canary-deploys.md)

<details>
<summary>All deploy features</summary>

- Webhook deploys from GitHub, GitLab, Bitbucket, Gitea and Forgejo ([Git integrations](docs/git-integrations.md))
- Dockerfile, Compose and Railpack (beta) builds with a build queue ([Build Center](docs/build-center.md))
- Previews: stable on GitHub, beta on GitLab and Bitbucket
- Approvals and deploy freezes ([Deploy safety](docs/deploy-safety.md))
- Functions in Node, Python or Go (beta) ([Functions](docs/functions.md))

</details>

**[One-click templates](docs/template-catalog.md)**
- 565 active templates in 22 categories, 574 entries in all
- 346 with a recorded real-engine pass, 228 marked preview ([how templates work](docs/templates-and-registry.md))

**[Automatic HTTPS and edge](docs/domains-and-ingress.md)**
- Automatic HTTPS with HTTP-01 and DNS-01 wildcards
- [Firewall, bot lists and attack mode](docs/firewall.md)
- [Per-app rate limits](docs/abuse-defense.md) (beta)
- [Private networks and WireGuard mesh](docs/multi-node.md) (beta)

**[Spin up nodes on any cloud](docs/node-provisioning.md)**
- Provision nodes at six clouds (beta)
- [Join any Linux host](docs/add-node-anywhere.md) with a single-use token
- [DNS and registries](docs/cloud-integrations.md) at Cloudflare, Route53, ECR, ACR
- Kubernetes: [works with the clusters and cloud accounts you already have](docs/import-kubernetes.md) (experimental)

**[Managed databases and backups](docs/managing-databases.md)**
- 13 database engines: 9 live-verified, 4 built but not yet live-verified
- [Backups to S3-compatible, R2 or local](docs/backups-and-storage.md) targets
- [Backups with restore drills](docs/prove-your-backups-work.md)
- Point-in-time recovery for Postgres

<details>
<summary>All data and backup features</summary>

- 13 engines in code. Built but not yet live-verified: Meilisearch, Qdrant, Typesense, Memcached
- Backup targets: S3-compatible, R2 or local ([Backups and storage](docs/backups-and-storage.md))
- Restore drills ([Prove your backups work](docs/prove-your-backups-work.md))
- Point-in-time recovery for Postgres

</details>

**[Built-in metrics, logs and alerts](docs/observability.md)**
- [Log search and archive](docs/log-explorer.md) with built-in metrics
- [OTLP ingest](docs/otlp-ingest.md)
- [Public status page](docs/status-page.md) and [alerts to 18 channel kinds](docs/integrations.md)

**[SSO, RBAC and audit log](docs/identity-and-access.md)**
- Passkeys, TOTP and [OAuth with 20 providers plus OIDC](docs/sso-oidc.md)
- [SAML and SCIM](docs/sso-saml.md) (beta, off by default)
- [IAM policies and teams](docs/teams-and-roles.md)
- [Hash-chained audit log](docs/audit-log.md)

**[CLI, API, Terraform and MCP](docs/cli-reference.md)**
- [Terraform and OpenTofu provider](docs/terraform-provider.md)
- [MCP server with 259 tools](docs/mcp.md) for coding agents
- [HTTP API](docs/api-reference.md) and `levelrail-cli`
- [YAML platform as code](docs/platform-as-code.md) (experimental)

[All features, with maturity labels](docs/features.md)

Footprint: about 200 MB idle RAM and 1.5% of one CPU on a 2 vCPU box ([measured, method included](docs/benchmarks.md)). Runs on Linux amd64 and arm64 with systemd and Docker.

## Install

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

Needs Linux (amd64 or arm64) with systemd and root; the installer adds Docker. Open ports 80 and 443, open the setup link it prints, choose a password, then deploy from the CLI (the dashboard's **Connect your CLI** card gives you the login line; pass `--api-url` always, and `--profile NAME` if you manage more than one server):

```bash
levelrail-cli login --api-url http://127.0.0.1:8080 --code ABCD-EF23
levelrail-cli import https://github.com/your-org/your-app --deploy
```


<table>
  <tr>
    <td width="50%" valign="top" align="center">
      <img src="docs/assets/demos/install-deploy.gif" alt="Terminal recording: install Levelrail, then deploy an app that goes live over HTTPS" width="440"><br>
      <sub><b>Install, then deploy</b></sub>
    </td>
    <td width="50%" valign="top" align="center">
      <img src="docs/assets/demos/deploy-rollback.gif" alt="Terminal recording: deploy with a readiness gate, then roll back to the previous image" width="440"><br>
      <sub><b>Gated deploy, one-step rollback</b></sub>
    </td>
  </tr>
</table>


[First 15 minutes](docs/first-15-minutes.md) &middot; [Installing](docs/installing.md) (Docker, pinned version, source build, troubleshooting) &middot; [CLI only](docs/installing.md#installing-just-the-cli) (`brew install glincker/tap/levelrail-cli` or `npx levelrail-cli`) &middot; [Free Arm VM](docs/install-on-oracle-cloud.md) &middot; [Migrate from Coolify, Dokploy, CapRover or Dokku](docs/migrate-to-levelrail.md) &middot; [Verify signed releases](docs/installing.md#verifying-release-binaries)

Using a coding agent? `npx skills add https://github.com/glincker/levelrail --skill levelrail`, then ask it to deploy your repo. See [Agents](docs/mcp.md).

### Install with your coding agent

Paste this into Claude Code, Cursor or any agent that can run shell commands on your server:

```text
Install Levelrail on this server: run `curl -fsSL https://levelrail.com/install.sh | sudo sh`.
Open the setup link it prints and tell me, because I choose the password myself.
Then run `levelrail-cli doctor` and `levelrail-cli status` and report anything that fails.
Do not write any token or password to a file or into this chat.
```

<table>
  <tr>
    <td width="33%" valign="top"><a href="docs/product-tour.mdx"><picture><source media="(prefers-color-scheme: dark)" srcset="docs/assets/screenshots/dashboard-home.png"><img src="docs/assets/screenshots/dashboard-home-light.png" alt="Levelrail dashboard home: fleet health, requests and recent deploys" loading="lazy"></picture></a><br><sub><b>The control plane at a glance</b></sub></td>
    <td width="33%" valign="top"><a href="docs/deploying-apps.md"><picture><source media="(prefers-color-scheme: dark)" srcset="docs/assets/screenshots/apps-list.png"><img src="docs/assets/screenshots/apps-list-light.png" alt="Levelrail apps list with status, traffic and last deploy" loading="lazy"></picture></a><br><sub><b>Every app, its health and its last deploy</b></sub></td>
    <td width="33%" valign="top"><a href="docs/deploying-apps.md"><picture><source media="(prefers-color-scheme: dark)" srcset="docs/assets/screenshots/deploy-history.png"><img src="docs/assets/screenshots/deploy-history-light.png" alt="Levelrail deploy history with promote and logs" loading="lazy"></picture></a><br><sub><b>Deploy history, promote and rollback</b></sub></td>
  </tr>
  <tr>
    <td width="33%" valign="top"><a href="docs/product-tour.mdx"><picture><source media="(prefers-color-scheme: dark)" srcset="docs/assets/screenshots/app-overview.png"><img src="docs/assets/screenshots/app-overview-light.png" alt="Levelrail app metrics: CPU, memory and deploy markers" loading="lazy"></picture></a><br><sub><b>Metrics and logs, no Grafana stack</b></sub></td>
    <td width="33%" valign="top"><a href="docs/multi-node.md"><picture><source media="(prefers-color-scheme: dark)" srcset="docs/assets/screenshots/nodes.png"><img src="docs/assets/screenshots/nodes-light.png" alt="Levelrail nodes list with health and capacity" loading="lazy"></picture></a><br><sub><b>Nodes and their capacity</b></sub></td>
    <td width="33%" valign="top"><a href="docs/deploying-apps.md"><picture><source media="(prefers-color-scheme: dark)" srcset="docs/assets/screenshots/new-app-wizard.png"><img src="docs/assets/screenshots/new-app-wizard-light.png" alt="Levelrail New app page: start from a Git repository, template, container image or sample app" loading="lazy"></picture></a><br><sub><b>New app from a repo, template or image</b></sub></td>
  </tr>
</table>

All screenshots are captured from a real running control plane. More in the [Product tour](https://docs.levelrail.com/product-tour?utm_source=github&utm_medium=readme&utm_campaign=tour).

## Who it is for

**Whoever you are, you start with one line.**

| You are | You get | Start |
| --- | --- | --- |
| A solo developer with one VPS | Git deploys, HTTPS in one click and about 200 MB idle | `curl -fsSL https://levelrail.com/install.sh \| sudo sh` |
| Building with Claude Code, Cursor or another coding agent | An MCP server that deploys, waits, diagnoses and rolls back inside the token's limits | `npx skills add https://github.com/glincker/levelrail --skill levelrail` |
| An agency or team | Approvals, deploy freezes, IAM policies, a verifiable audit log, OIDC sign-in | [Identity and access](docs/identity-and-access.md) |
| On Coolify, Dokploy, CapRover or Dokku today | A dry run first, and the old server keeps serving | `levelrail-cli import platform coolify --url https://coolify.example.com --dry-run` |
| Running a homelab or a free Arm VM | arm64 builds and a minimal profile | [Server profiles](docs/profiles.md), [Arm VM](docs/install-on-oracle-cloud.md) |

## Built for coding agents

**Let your agent ship. You keep the keys.**

```text
Deploy this repo to my Levelrail and tell me when it is live.
```

- **Inside the token's limits.** The tool list follows the abilities of the token: a read token never sees a deploy tool, and `--readonly` refuses to start with a write token.
- **Writes need a confirmation.** Mutations are confirm-gated and off until you pass `--allow-write`.
- **Cheap to load.** About 2.4k tokens of tools by default, 240+ on demand in toolsets ([tool surface](docs/mcp-tool-surface.md)).
- **Audited.** Every call is recorded with the caller surface (CLI, dashboard, MCP or API).
- **Not in the control loop.** AI never sits in the reconcile path, so a wrong guess cannot wedge your servers.
- **A skill, not just tools.** It deploys, waits for the rollout to converge, reads the failure and rolls back ([skill](skills/levelrail/SKILL.md), [MCP](docs/mcp.md), [threat model](docs/mcp.md#threat-model)).

## vs Coolify, Dokploy, CapRover and Dokku

**Same job, fewer moving parts, more proof.**

| | Levelrail | Coolify | Dokploy | CapRover | Dokku |
| --- | --- | --- | --- | --- | --- |
| **Agent access (MCP)** | Read and write, ability-scoped, confirm-gated, audited | Official server, read-only | MCP server repo exists | Not documented | Not documented |
| **Switching in** | Dry-run importers for all four, old server keeps serving | n/a | n/a | n/a | n/a |
| Node control | Reverse-dialed agent | SSH + CLI | SSH + Engine API | Swarm API | Local bash |
| Built-in metrics and logs | Yes | Opt-in add-on | Separate binary | No | No |
| Rollback | Pinned images, readiness-gated | Retained images | Swarm rollback | Manual | Default scheduler: none |
| Backup verification | Automatic | Non-empty check | None | None | No backups |
| Audit log | Hash-chained, verifiable | Not documented | Not documented | Not documented | Not documented |

Cells state only what each project's public docs say (last checked 2026-11-07), and "Not documented" means we found nothing, not that it is missing. Coolify's MCP: [their docs](https://coolify.io/docs/integrations/mcp). This is positioning, not a ranking. Levelrail is the youngest project here, with fewer users and less production mileage. Sourced details: [Comparison](docs/comparison.md).

## Production ready, with receipts

| Claim | Evidence |
| --- | --- |
| Every feature labelled stable or beta | [Feature status](docs/feature-status.md) |
| Survives failures | [Resilience](docs/resilience.md): disk full, link loss, database damage, safe-mode boot |
| Measured, not promised | [Benchmarks](docs/benchmarks.md) |
| Tested against real containers | [CI](.github/workflows/ci.yml) and [`test/e2e`](test/e2e) |
| Signed releases | Sigstore signature and provenance attestation, [verify them](docs/installing.md#verifying-release-binaries) |
| Honest about gaps | [Known limits](docs/known-limits.md) |

## How it works

```mermaid
flowchart LR
  C["Dashboard, CLI, MCP server"] --> API
  subgraph CP["Control plane (one Go binary)"]
    API["HTTP API"] --> R["Level-triggered reconciler"]
    R --> S[("SQLite state")]
    O["Built-in metrics and logs"]
    I["Embedded Caddy ingress and TLS"]
  end
  R -->|"gRPC, mTLS"| A["Node agents (dial out)"]
  A --> D["Docker Engine API"]
  I --> D
```

Details: [Architecture](docs/architecture.md).

## Guides

[Next.js with Postgres](https://github.com/glincker/levelrail/discussions/1239) &middot; [Django with Postgres](https://github.com/glincker/levelrail/discussions/1243) &middot; [Push to deploy with GitHub](https://github.com/glincker/levelrail/discussions/1244) &middot; [How deploys stay safe](https://github.com/glincker/levelrail/discussions/1246) &middot; [Scale to zero on a small VPS](https://github.com/glincker/levelrail/discussions/1242)

Maintain an open-source app? Add a one-click button: [Deploy button](docs/deploy-button.md).

[![Deploy to Levelrail](docs/assets/brand/deploy-button-split.svg)](docs/deploy-button.md)

## FAQ

**Does it need Kubernetes?** No. The control plane is one Go binary with a Docker runtime. If you already run Kubernetes, you can connect a cluster and deploy to it (experimental, off by default).

**What does it cost?** Nothing. Apache 2.0, one edition, no license key. You pay for your own servers ([Pricing](docs/pricing.md)).

**Can I migrate from Coolify?** Yes. `levelrail-cli import platform coolify --url https://coolify.example.com --dry-run` (with the source token in `APP_IMPORT_SOURCE_TOKEN`) shows what would be created first, and the old server keeps serving ([Migrate](docs/migrate-to-levelrail.md)).

**How do I upgrade?** Settings > Updates or `levelrail-cli upgrade --plan`. It verifies the release, snapshots your data and rolls back by itself if the new release is unhealthy ([Upgrade safely](docs/upgrade-safely.md)).

## Community

[Discussions](https://github.com/glincker/levelrail/discussions) &middot; [Issues](https://github.com/glincker/levelrail/issues) &middot; [Discord](https://discord.gg/Ar5pcaZB99) &middot; [support@levelrail.com](mailto:support@levelrail.com) &middot; [Security policy](.github/SECURITY.md) &middot; [Contributing](.github/CONTRIBUTING.md) &middot; [llms.txt](llms.txt) and [AGENTS.md](AGENTS.md) for AI agents

## Star the repo

If Levelrail saves you a weekend, a [star](https://github.com/glincker/levelrail) helps other self-hosters find it.

<a href="https://github.com/glincker/levelrail/graphs/contributors"><img src="https://contrib.rocks/image?repo=glincker/levelrail" alt="Levelrail contributors"></a>

[![Star History Chart](https://api.star-history.com/svg?repos=glincker/levelrail&type=Date)](https://star-history.com/#glincker/levelrail&Date)

Apache 2.0, see [LICENSE](LICENSE). Built by Gagan Deep Singh of [GLINR STUDIOS](https://glinr.com) and [theSVG](https://thesvg.org).

<div align="center">

<a href="https://glinr.com">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/brand/glincker-light.svg">
    <img src="docs/assets/brand/glincker-dark.svg" alt="GLINR STUDIOS" width="56">
  </picture>
</a>

<sub>Powered by <a href="https://glinr.com"><b>GLINR STUDIOS</b></a> &middot; icon from <a href="https://thesvg.org/icon/glincker">theSVG</a></sub>

</div>
