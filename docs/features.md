---
title: "All features"
description: Every Levelrail feature by area, with a docs link and a stable, beta or experimental label for each.
"og:url": "https://docs.levelrail.com/features"
---

Levelrail is a lightweight, self-hosted deployment platform: one Go binary whose control plane idles at about 200 to 220 MB of RAM and 1.5 percent of one CPU on a 2 vCPU box ([Performance](/performance), [Benchmarks](/benchmarks)). This page lists what the 1.4.0 tree contains, grouped into eight areas. Labels follow [Feature status](/feature-status): a feature with no label there is core and on by default. For a shorter view, see the [README](https://github.com/glincker/levelrail#what-you-get).

## Ship with git push (deploy)

Guide: [Deploying apps](/deploying-apps).

- Git push or webhook deploys from GitHub, GitLab, Bitbucket, Gitea and Forgejo. See [Git integrations](/git-integrations) and [Git push deploy](/git-push-deploy).
- Dockerfile, Compose and Railpack builds (Railpack is beta) with a build queue. See [Build Center](/build-center).
- Readiness-gated rollouts and one-step rollback. See [Zero-downtime deploys](/zero-downtime-deploys) and [Rollback](/rollback).
- Canaries and split tests. See [Canary deploys](/canary-deploys) and [Split testing](/split-testing).
- Preview environments per pull request: stable on GitHub, beta on GitLab and Bitbucket. See [Deploy previews](/deploy-previews).
- Deploy approvals and deploy freezes. See [Deploy safety](/deploy-safety).
- Functions in Node, Python or Go (beta). See [Functions](/functions).

## One-click templates

Guide: [Template catalog](/template-catalog). How they work: [Templates and registry](/templates-and-registry).

- 570+ templates in the catalog. Exact split: 574 catalog entries (565 active, 9 deprecated) in 22 categories.
- 346 entries have a recorded real-engine pass.
- 228 entries are marked preview: installable, with no recorded pass yet.

## Automatic HTTPS and edge (networking)

Guide: [Domains and ingress](/domains-and-ingress).

- Embedded ingress with automatic TLS through HTTP-01 and DNS-01 wildcards.
- Per-domain headers, forwarders, geo rules, caching and redirects. See [Domain traffic controls](/domain-traffic-controls).
- Firewall rules, IP lists, browser challenge, bot lists and attack mode. See [Firewall](/firewall).
- Per-app rate limits and deny lists (beta). See [Abuse defense](/abuse-defense).
- Round-robin balancing across replicas. See [Load balancing](/load-balancing).
- Private networks and a WireGuard mesh (beta). See [Multi-node](/multi-node).

## Spin up nodes on any cloud

Guide: [Node provisioning](/node-provisioning).

- Provision nodes at Hetzner, DigitalOcean, AWS, Azure, GCP or Oracle Cloud (beta).
- Join any Linux host with a single-use token over mTLS. See [Add a node anywhere](/add-node-anywhere).
- DNS at Cloudflare and Route53, image registries on ECR, Artifact Registry and ACR. See [Cloud integrations](/cloud-integrations) for per-cloud status.
- Works with the clusters and cloud accounts you already have (experimental): connect existing Kubernetes clusters and deploy to them, off by default. See [Kubernetes clusters](/kubernetes-clusters) and [Import Kubernetes manifests](/import-kubernetes).

## Managed databases and backups

Guide: [Managing databases](/managing-databases).

- 13 database engines in code. 9 are live-verified. 4 (Meilisearch, Qdrant, Typesense, Memcached) are built but not yet live-verified.
- Backups to S3-compatible, R2 or local targets. See [Backups and storage](/backups-and-storage).
- Restore drills that prove a backup works. See [Prove your backups work](/prove-your-backups-work).
- Point-in-time recovery for Postgres.

## Built-in metrics, logs and alerts (observability)

Guide: [Observability](/observability).

- Built-in metrics, log search and log archive. See [Log explorer](/log-explorer) and [Log archive](/log-archive).
- OTLP ingest. See [OTLP ingest](/otlp-ingest).
- Public status page. See [Status page](/status-page).
- Deploy traces and traffic export. See [Deploy trace](/deploy-trace) and [Traffic export](/traffic-export).
- Alerts to 18 notification channel kinds. See [Alerts](/alerts).
- HTTP, TCP, heartbeat and TLS uptime monitors (experimental). See [Monitors](/monitors).

## SSO, RBAC and audit log (security and access)

Guide: [Identity and access](/identity-and-access).

- Passwords, passkeys and TOTP.
- OAuth sign-in with 20 providers plus generic OIDC. See [OIDC](/sso-oidc).
- SAML and SCIM (beta, off by default). See [SAML and SCIM](/sso-saml).
- IAM policies and teams. See [Teams and roles](/teams-and-roles).
- Hash-chained audit log. See [Audit log](/audit-log).
- Encrypted secrets. See [Environment and secrets](/environment-and-secrets).
- Signed releases. See [Verify a release](/verify-a-release).

## CLI, API, Terraform and MCP (automation)

Guide: [CLI reference](/cli-reference).

- `levelrail-cli` and an HTTP API. See [API reference](/api-reference).
- Terraform and OpenTofu provider. See [Terraform provider](/terraform-provider).
- YAML platform as code (experimental). See [Platform as code](/platform-as-code).
- CI pipelines. See [Pipelines](/pipelines).
- 11 per-app integrations: Sentry, PostHog, GlitchTip, OneUptime, Plausible, Datadog, Axiom, Better Stack, LogSnag, Bugsnag and New Relic. See [Integrations](/integrations).
- Signed plugins for notification channels. See [Plugins](/plugins).
- 259 MCP tools for coding agents. See [MCP](/mcp) and [Tool surface](/mcp-tool-surface).
- A mail server template with mailboxes and DNS records (experimental). See [Mail template](/mail-template).

## Footprint

About 200 MB idle RAM and 1.5% of one CPU on a 2 vCPU box, measured with the method included in [Benchmarks](/benchmarks). Runs on Linux amd64 and arm64 with systemd and Docker.
