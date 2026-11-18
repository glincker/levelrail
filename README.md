<div align="center">
<img src="docs/assets/brand/levelrail-default.svg" alt="Levelrail logo" width="56" height="56">
<h1>Levelrail</h1>
<p><strong>Git push to production on your own servers.</strong><br>
A self-hosted deployment platform in one Go binary. The control plane idles at about 200 MB of RAM (<a href="docs/performance.md">measured</a>), with 570+ one-click apps, rollbacks, previews, HTTPS and databases built in.</p>
<p><code>curl -fsSL https://levelrail.com/install.sh | sudo sh</code> &middot; <a href="https://demo.levelrail.com"><b>Try the live demo</b></a></p>
<p>
  <a href="https://github.com/glincker/levelrail/stargazers"><img src="https://img.shields.io/github/stars/glincker/levelrail?style=flat&logo=github" alt="GitHub stars"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/glincker/levelrail" alt="License: Apache 2.0"></a>
  <a href="https://github.com/glincker/levelrail/releases"><img src="https://img.shields.io/github/v/release/glincker/levelrail?include_prereleases&label=release" alt="Latest release"></a>
  <a href="https://github.com/glincker/levelrail/actions/workflows/ci.yml"><img src="https://github.com/glincker/levelrail/actions/workflows/ci.yml/badge.svg" alt="CI status"></a>
  <a href="https://demo.levelrail.com"><img src="https://img.shields.io/badge/demo-demo.levelrail.com-2f6feb" alt="Live demo"></a>
</p>
<p>
  <a href="https://levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">Website</a> &middot;
  <a href="https://docs.levelrail.com?utm_source=github&amp;utm_medium=readme&amp;utm_campaign=nav">Docs</a> &middot;
  <a href="#quickstart">Quickstart</a> &middot;
  <a href="#vs-coolify-dokploy-caprover-and-dokku">Compare</a> &middot;
  <a href="docs/faq.md">FAQ</a> &middot;
  <a href="docs/roadmap.md">Roadmap</a> &middot;
  <a href="https://github.com/glincker/levelrail/discussions">Discussions</a>
</p>
<img src="docs/assets/demos/product-tour.gif" alt="Levelrail dashboard tour: fleet overview, apps list, deploy history and built-in metrics" width="880">
</div>

Levelrail turns Linux servers into a private cloud and works with the Kubernetes clusters and cloud accounts you already have. Single-server is production ready, multi-node is beta. Every feature carries a stable, beta or experimental label in [Feature status](docs/feature-status.md).

## Quickstart

You need a Linux server (amd64 or arm64) with systemd and root. The installer adds Docker. Open ports 80 and 443.

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

1. Open the setup link the installer prints and choose a password. That creates the admin account.
2. In the dashboard, copy the login line from the **Connect your CLI** card and run it (it looks like `levelrail-cli login --api-url http://127.0.0.1:8080 --code ABCD-EF23`).
3. Deploy your first app:

```bash
levelrail-cli import https://github.com/your-org/your-app --deploy
```

If something fails, run `levelrail-cli doctor` and see [Installing](docs/installing.md#troubleshooting). Next: [First 15 minutes](docs/first-15-minutes.md). CLI only: `brew install glincker/tap/levelrail-cli` or `npx levelrail-cli --help`.

Want a look first? [Try the live demo](https://demo.levelrail.com), a hosted read-only demo of the dashboard with fictional data ([how it runs](docs/run-a-read-only-demo.md)).

## Where you can use it

| Surface | Start | Docs |
| --- | --- | --- |
| Web dashboard | Open the setup link the installer prints | [First 15 minutes](docs/first-15-minutes.md) |
| CLI | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/apple/default.svg" alt="Apple" title="Apple" width="20" height="20"> `brew install glincker/tap/levelrail-cli`, `npx levelrail-cli --help` or `npm install --global levelrail-cli` | [Installing](docs/installing.md) |
| HTTP API | Bearer token, `/api/v1` | [API reference](docs/api-reference.md) |
| Terraform and OpenTofu | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/terraform/default.svg" alt="Terraform" title="Terraform" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/opentofu/default.svg" alt="OpenTofu" title="OpenTofu" width="20" height="20"> `source = "glincker/levelrail"` | [Terraform provider](docs/terraform-provider.md) |
| MCP and coding agents | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/model-context-protocol/default.svg" alt="MCP" title="MCP" width="20" height="20"> 259 tools | [MCP](docs/mcp.md) |
| Mobile API (beta, off by default) | Enable in Settings, Beta features | [Mobile API](docs/mobile-api.md) |
| Desktop runtime on Mac (experimental) | Private Linux VM on your Mac | [Desktop runtime](docs/desktop-runtime.md) |

## What you get

Maturity follows [Feature status](docs/feature-status.md).

| Area | In one line | Works with |
| --- | --- | --- |
| [Deploy](docs/deploying-apps.md) | Git push from GitHub, GitLab, Bitbucket, Gitea or Forgejo, readiness-gated rollouts, one-step rollback, previews and canaries | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/bitbucket/default.svg" alt="Bitbucket" title="Bitbucket" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitea/default.svg" alt="Gitea" title="Gitea" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/docker/default.svg" alt="Docker" title="Docker" width="20" height="20"> |
| [Templates](docs/template-catalog.md) | 570+ templates in the catalog, 346 with a recorded real-engine pass | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="20" height="20"> |
| [Edge and HTTPS](docs/domains-and-ingress.md) | Automatic certificates including wildcards, firewall, bot lists, per-domain headers and redirects | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/letsencrypt/default.svg" alt="Let's Encrypt" title="Let's Encrypt" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/cloudflare/default.svg" alt="Cloudflare" title="Cloudflare" width="20" height="20"> |
| [Nodes and clouds](docs/node-provisioning.md) | Join any Linux host with a token, provision nodes at 6 clouds (beta), existing Kubernetes clusters as targets (experimental) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/hetzner/default.svg" alt="Hetzner" title="Hetzner" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/digitalocean/default.svg" alt="DigitalOcean" title="DigitalOcean" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/aws/default.svg" alt="AWS" title="AWS" width="20" height="20"> |
| [Databases and backups](docs/managing-databases.md) | 13 engines, backups to S3-compatible, R2 or local disk, [restore drills](docs/prove-your-backups-work.md) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mysql/default.svg" alt="MySQL" title="MySQL" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mariadb/default.svg" alt="MariaDB" title="MariaDB" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/mongodb/default.svg" alt="MongoDB" title="MongoDB" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/aws-amazon-simple-storage-service/default.svg" alt="Amazon S3" title="Amazon S3" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/minio/default.svg" alt="MinIO" title="MinIO" width="20" height="20"> |
| [Observability](docs/observability.md) | Built-in metrics, log search, alerts to 18 channel kinds, OTLP ingest, status page. Grafana reads logs through a [Loki-compatible API](docs/loki-api.md) | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/prometheus/default.svg" alt="Prometheus" title="Prometheus" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/slack/default.svg" alt="Slack" title="Slack" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/discord/default.svg" alt="Discord" title="Discord" width="20" height="20"> |
| [Security and access](docs/identity-and-access.md) | OAuth and OIDC sign-in, passkeys, TOTP, IAM policies, hash-chained audit log | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="20" height="20"> |
| [Automation](docs/cli-reference.md) | CLI, HTTP API, [Terraform and OpenTofu provider](docs/terraform-provider.md), 259 [MCP tools](docs/mcp.md) for coding agents | <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/terraform/default.svg" alt="Terraform" title="Terraform" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/opentofu/default.svg" alt="OpenTofu" title="OpenTofu" width="20" height="20"> <img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/model-context-protocol/default.svg" alt="MCP" title="MCP" width="20" height="20"> |

<details>
<summary>All deploy features</summary>

- Git push or webhook deploys from GitHub, GitLab, Bitbucket, Gitea and Forgejo. See [Git integrations](docs/git-integrations.md) and [Git push deploy](docs/git-push-deploy.md).
- Dockerfile, Compose and Railpack builds (Railpack is beta) with a build queue. See [Build Center](docs/build-center.md).
- Readiness-gated rollouts and one-step rollback. See [Zero-downtime deploys](docs/zero-downtime-deploys.md) and [Rollback](docs/rollback.md).
- Canaries and split tests. See [Canary deploys](docs/canary-deploys.md) and [Split testing](docs/split-testing.md).
- Preview environments per pull request: stable on GitHub, beta on GitLab and Bitbucket. See [Deploy previews](docs/deploy-previews.md).
- Deploy approvals and deploy freezes. See [Deploy safety](docs/deploy-safety.md).
- Functions in Node, Python or Go (beta). See [Functions](docs/functions.md).
</details>
<details>
<summary>All template details</summary>

- 574 catalog entries (565 active, 9 deprecated) in 22 categories.
- 346 entries have a recorded real-engine pass.
- 228 entries are marked preview: installable, with no recorded pass yet.
</details>
<details>
<summary>All edge features</summary>

- Embedded ingress with automatic TLS through HTTP-01 and DNS-01 wildcards.
- Per-domain headers, forwarders, geo rules, caching and redirects. See [Domain traffic controls](docs/domain-traffic-controls.md).
- Firewall rules, IP lists, browser challenge, bot lists and attack mode. See [Firewall](docs/firewall.md).
- Per-app rate limits and deny lists (beta). See [Abuse defense](docs/abuse-defense.md).
- Round-robin balancing across replicas. See [Load balancing](docs/load-balancing.md).
- Private networks and a WireGuard mesh (beta). See [Multi-node](docs/multi-node.md).
</details>
<details>
<summary>All node and cloud features</summary>

- Provision nodes at Hetzner, DigitalOcean, AWS, Azure, GCP or Oracle Cloud (beta).
- Join any Linux host with a single-use token over mTLS. See [Add a node anywhere](docs/add-node-anywhere.md).
- DNS at Cloudflare and Route53, image registries on ECR, Artifact Registry and ACR. See [Cloud integrations](docs/cloud-integrations.md) for per-cloud status.
- Works with the clusters and cloud accounts you already have (experimental): connect existing Kubernetes clusters and deploy to them, off by default. See [Kubernetes clusters](docs/kubernetes-clusters.md).
</details>
<details>
<summary>All data and backup features</summary>

- 13 database engines in code. 9 are live-verified. 4 (Meilisearch, Qdrant, Typesense, Memcached) are built but not yet live-verified.
- Backups to S3-compatible, R2 or local targets. See [Backups and storage](docs/backups-and-storage.md).
- Restore drills that prove a backup works. See [Prove your backups work](docs/prove-your-backups-work.md).
- Point-in-time recovery for Postgres.
</details>
<details>
<summary>All observability features</summary>

- Built-in metrics, log search and log archive. See [Log explorer](docs/log-explorer.md) and [Log archive](docs/log-archive.md).
- OTLP ingest. See [OTLP ingest](docs/otlp-ingest.md).
- Public status page. See [Status page](docs/status-page.md).
- Deploy traces and traffic export. See [Deploy trace](docs/deploy-trace.md) and [Traffic export](docs/traffic-export.md).
- Alerts to 18 notification channel kinds. See [Alerts](docs/alerts.md).
- HTTP, TCP, heartbeat and TLS uptime monitors (experimental). See [Monitors](docs/monitors.md).
</details>
<details>
<summary>All security and access features</summary>

- Passwords, passkeys and TOTP.
- OAuth sign-in with 20 providers plus generic OIDC. See [OIDC](docs/sso-oidc.md).
- SAML and SCIM (beta, off by default). See [SAML and SCIM](docs/sso-saml.md).
- IAM policies and teams. See [Teams and roles](docs/teams-and-roles.md).
- Hash-chained audit log. See [Audit log](docs/audit-log.md).
- Encrypted secrets. See [Environment and secrets](docs/environment-and-secrets.md).
- Signed releases. See [Verify a release](docs/verify-a-release.md).
</details>
<details>
<summary>All automation features</summary>

- `levelrail-cli` and an HTTP API. See [API reference](docs/api-reference.md).
- Terraform and OpenTofu provider. See [Terraform provider](docs/terraform-provider.md), the [Terraform registry](https://registry.terraform.io/providers/glincker/levelrail/latest) and the [OpenTofu registry](https://search.opentofu.org/provider/glincker/levelrail/latest).
- YAML platform as code (experimental). See [Platform as code](docs/platform-as-code.md).
- CI pipelines. See [Pipelines](docs/pipelines.md).
- 11 per-app integrations: Sentry, PostHog, GlitchTip, OneUptime, Plausible, Datadog, Axiom, Better Stack, LogSnag, Bugsnag and New Relic. See [Alerts](docs/alerts.md).
- Signed plugins for notification channels. See [Plugins](docs/plugins.md).
- 259 MCP tools for coding agents. See [MCP](docs/mcp.md) and [Tool surface](docs/mcp-tool-surface.md).
- A mail server template with mailboxes and DNS records (experimental). See [Mail template](docs/mail-template.md).
</details>

All features, with maturity labels: [docs/features.md](docs/features.md).

## Built for coding agents

259 [MCP tools](docs/mcp.md), scoped to the token's abilities, confirm-gated for writes and audited. Add the skill with `npx skills add https://github.com/glincker/levelrail --skill levelrail`, then ask your agent to deploy your repo. To install with an agent, paste this into Claude Code, Cursor or any agent that can run shell commands on your server:

```text
Install Levelrail on this server: run `curl -fsSL https://levelrail.com/install.sh | sudo sh`.
Open the setup link it prints and tell me, because I choose the password myself.
Then run `levelrail-cli doctor` and `levelrail-cli status` and report anything that fails.
Do not write any token or password to a file or into this chat.
```

## Production ready, with receipts

| Claim | Evidence |
| --- | --- |
| Every feature labelled stable or beta | [Feature status](docs/feature-status.md) |
| Survives failures | [Resilience](docs/resilience.md) |
| Measured, not promised | [Benchmarks](docs/benchmarks.md) |
| Tested against real containers | [CI](.github/workflows/ci.yml) and [`test/e2e`](test/e2e) |
| Signed releases | [Verify them](docs/installing.md#verifying-release-binaries) |
| Honest about gaps | [Known limits](docs/known-limits.md) |

## vs Coolify, Dokploy, CapRover and Dokku

| | Levelrail | Coolify | Dokploy | CapRover | Dokku |
| --- | --- | --- | --- | --- | --- |
| **Agent access (MCP)** | Read and write, ability-scoped, confirm-gated, audited | Official server, read-only | MCP server repo exists | Not documented | Not documented |
| **Switching in** | Dry-run importers for all four, old server keeps serving | n/a | n/a | n/a | n/a |
| Node control | Reverse-dialed agent | SSH + CLI | SSH + Engine API | Swarm API | Local bash |
| Built-in metrics and logs | Yes | Opt-in add-on | Separate binary | No | No |
| Rollback | Pinned images, readiness-gated | Retained images | Swarm rollback | Manual | Default scheduler: none |
| Backup verification | Automatic | Non-empty check | None | None | No backups |
| Audit log | Hash-chained, verifiable | Not documented | Not documented | Not documented | Not documented |

Cells state only what each project's public docs say (last checked 2026-11-07), and "Not documented" means we found nothing, not that it is missing. This is positioning, not a ranking. Levelrail is the youngest project here, with fewer users and less production mileage. Sourced details: [Comparison](docs/comparison.md).

## Community

[Discussions](https://github.com/glincker/levelrail/discussions) &middot; [Issues](https://github.com/glincker/levelrail/issues) &middot; [Discord](https://discord.gg/Ar5pcaZB99) &middot; [support@levelrail.com](mailto:support@levelrail.com) &middot; [Security policy](.github/SECURITY.md)

If Levelrail saves you a weekend, a [star](https://github.com/glincker/levelrail) helps other self-hosters find it.

## License

Apache 2.0, see [LICENSE](LICENSE). Built by Gagan Singh aka GDS K S of [GLINR Studios](https://glinr.com) and [theSVG](https://thesvg.org).

<div align="center">

<!-- i18n-nav:start -->
<p>English &middot; <a href="docs/i18n/README.zh-Hans.md">简体中文</a> &middot; <a href="docs/i18n/README.es.md">Español</a> &middot; <a href="docs/i18n/README.pt-BR.md">Português (BR)</a> &middot; <a href="docs/i18n/README.de.md">Deutsch</a> &middot; <a href="docs/i18n/README.fr.md">Français</a> &middot; <a href="docs/i18n/README.ja.md">日本語</a></p>
<!-- i18n-nav:end -->

</div>
