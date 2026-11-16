---
title: "Levelrail documentation"
icon: "/assets/icons/house.svg"
sidebarTitle: "What is Levelrail"
description: Self-hosted deployment platform. Git deploys, one-click templates, edge and TLS, cloud nodes, managed databases and built-in observability. One Go binary.
"og:image": "https://docs.levelrail.com/public/og/index.png"
"twitter:image": "https://docs.levelrail.com/public/og/index.png"
"og:url": "https://docs.levelrail.com/index"
---

Levelrail turns one or more Linux servers into a private cloud. Push to a git repo and get a running app with TLS, logs, metrics and one-click rollback.

<CardGroup cols={2}>
  <Card title="Getting started" icon="/assets/icons/rocket-launch.svg" href="/getting-started">
    Install, sign in and deploy your first app in about ten minutes.
  </Card>
  <Card title="Deploying apps" icon="/assets/icons/cloud-arrow-up.svg" href="/deploying-apps">
    Dockerfile, Compose and Railpack builds with zero-downtime cutover.
  </Card>
  <Card title="CLI reference" icon="/assets/icons/terminal-window.svg" href="/cli-reference">
    Every command, flag and exit code.
  </Card>
  <Card title="Architecture" icon="/assets/icons/tree-structure.svg" href="/architecture">
    The reconciler, the node agent and how state flows.
  </Card>
</CardGroup>

## What is in the product

| Area | Contents |
| --- | --- |
| Deploy | Git deploys from GitHub, GitLab, Bitbucket, Gitea and Forgejo. Dockerfile, Compose and Railpack builds. Canaries, split tests, previews, approvals, rollback. See [Deploying apps](/deploying-apps). |
| Templates | 574 catalog entries, 346 with a recorded real-engine pass and 228 marked preview. See [Template catalog](/template-catalog). |
| Networking and edge | Embedded ingress, automatic TLS, per-domain traffic controls, firewall with browser challenge, per-app rate limits, replica balancing. See [Domains and ingress](/domains-and-ingress) and [Firewall](/firewall). |
| Cloud and nodes | Provision nodes at Hetzner, DigitalOcean, AWS, Azure, GCP or Oracle Cloud, or join any Linux host. Connect existing Kubernetes clusters (experimental, off by default). See [Node provisioning](/node-provisioning) and [Cloud integrations](/cloud-integrations). |
| Data and backups | 13 database engines in code (9 live-verified), backups to S3-compatible, R2 or local targets, restore drills and point-in-time recovery for Postgres. See [Managed databases](/managed-databases). |
| Observability | Metrics, logs, log archive, OTLP ingest, status page, alerts to 18 channel kinds. See [Monitors](/monitors) and [Status page](/status-page). |
| Automation | CLI, HTTP API, Terraform and OpenTofu provider, pipelines, 11 per-app integrations, 259 MCP tools. See [CLI reference](/cli-reference), [API reference](/api-reference) and [Terraform provider](/terraform-provider). |

Every feature is labelled stable, beta or experimental in [Feature status](/feature-status).

## Identity and access

- Password, passkeys, TOTP, OAuth with 20 providers plus generic OpenID Connect, IAM policies, teams and a hash-chained audit log. See [Identity and access](/identity-and-access).
- SAML single sign-on and SCIM provisioning are beta and off by default. See [SAML and SCIM](/sso-saml).

## Works with

Levelrail drives Docker directly and plugs into the tools you already use.

<div className="flex flex-wrap items-center gap-2 not-prose"><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/docker/default.svg" alt="Docker" title="Docker" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/caddy/default.svg" alt="Caddy" title="Caddy" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/letsencrypt/default.svg" alt="Let's Encrypt" title="Let's Encrypt" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/wireguard/default.svg" alt="WireGuard" title="WireGuard" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/minio/default.svg" alt="MinIO" title="MinIO" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="26" height="26" /></span></div>
