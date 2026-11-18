<div align="center">
<img src="docs/assets/brand/levelrail-default.svg" alt="Levelrail logo" width="56" height="56">
<h1>Levelrail</h1>
<p><strong>Lightweight, self-hosted deployment platform: git push to production on your own servers and cloud accounts.</strong><br>
The control plane is one Go binary that idles at about 200 MB of RAM ([measured idle footprint](docs/performance.md)). Apps you deploy use their own resources on top. Apache 2.0.</p>
<p>
  <a href="https://github.com/glincker/levelrail/stargazers"><img src="https://img.shields.io/github/stars/glincker/levelrail?style=flat&logo=github" alt="GitHub stars"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/glincker/levelrail" alt="License: Apache 2.0"></a>
  <a href="https://github.com/glincker/levelrail/releases"><img src="https://img.shields.io/github/v/release/glincker/levelrail?include_prereleases&label=release" alt="Latest release"></a>
  <a href="https://github.com/glincker/levelrail/actions/workflows/ci.yml"><img src="https://github.com/glincker/levelrail/actions/workflows/ci.yml/badge.svg" alt="CI status"></a>
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

## What you get

Maturity follows [Feature status](docs/feature-status.md).

| Area | In one line |
| --- | --- |
| [Deploy](docs/deploying-apps.md) | Git push from GitHub, GitLab, Bitbucket, Gitea or Forgejo, readiness-gated rollouts, one-step rollback, previews and canaries |
| [Templates](docs/template-catalog.md) | 570+ templates in the catalog, 346 with a recorded real-engine pass |
| [Edge and HTTPS](docs/domains-and-ingress.md) | Automatic certificates including wildcards, firewall, bot lists, per-domain headers and redirects |
| [Nodes and clouds](docs/node-provisioning.md) | Join any Linux host with a token, provision nodes at 6 clouds (beta), existing Kubernetes clusters as targets (experimental) |
| [Databases and backups](docs/managing-databases.md) | 13 engines, backups to S3-compatible, R2 or local disk, [restore drills](docs/prove-your-backups-work.md) |
| [Observability](docs/observability.md) | Built-in metrics, log search, alerts to 18 channel kinds, OTLP ingest, status page |
| [Security and access](docs/identity-and-access.md) | OAuth and OIDC sign-in, passkeys, TOTP, IAM policies, hash-chained audit log |
| [Automation](docs/cli-reference.md) | CLI, HTTP API, [Terraform and OpenTofu provider](docs/terraform-provider.md), 259 [MCP tools](docs/mcp.md) for coding agents |

All features, with maturity labels: [docs/features.md](docs/features.md).

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

Apache 2.0, see [LICENSE](LICENSE). Built by Gagan Deep Singh of [GLINR Studios](https://glinr.com) and [theSVG](https://thesvg.org).

<div align="center">

<!-- i18n-nav:start -->
<p>English &middot; <a href="docs/i18n/README.zh-Hans.md">简体中文</a> &middot; <a href="docs/i18n/README.es.md">Español</a> &middot; <a href="docs/i18n/README.pt-BR.md">Português (BR)</a> &middot; <a href="docs/i18n/README.de.md">Deutsch</a> &middot; <a href="docs/i18n/README.fr.md">Français</a> &middot; <a href="docs/i18n/README.ja.md">日本語</a></p>
<!-- i18n-nav:end -->

</div>
