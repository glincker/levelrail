---
title: "Levelrail documentation"
icon: "/assets/icons/house.svg"
sidebarTitle: "Introduction"
description: "Levelrail is an open source, self-hosted deployment platform in one Go binary: git deploys, templates, databases, TLS, backups, observability and MCP."
"og:image": "https://docs.levelrail.com/public/og/index.png"
"twitter:image": "https://docs.levelrail.com/public/og/index.png"
"og:url": "https://docs.levelrail.com/index"
---

**Levelrail is an open source (Apache 2.0), self-hosted deployment platform that runs as one Go binary and turns your own Linux servers or cloud accounts into a private cloud. It deploys apps from git or Docker images with automatic HTTPS, runs databases with backups, keeps logs and metrics, and exposes the same API to people, the CLI, Terraform and AI agents through MCP, with an audit log.**

The control plane idles at about 200 MB of RAM and 1.5 percent of one CPU on a 2 vCPU box (see [Performance](/performance) and [Benchmarks](/benchmarks)). Your apps use their own resources on top. There is one edition and no license key.

<CardGroup cols={3}>
  <Card title="What is Levelrail" icon="/assets/icons/house.svg" href="/what-is-levelrail">
    The definition, what it replaces and who it is for.
  </Card>
  <Card title="What it solves" icon="/assets/icons/scales.svg" href="/compared-with-other-approaches">
    Apps, databases, domains, TLS, backups and logs in one place. Compared by category.
  </Card>
  <Card title="Install" icon="/assets/icons/rocket-launch.svg" href="/getting-started">
    Install, sign in and deploy your first app in about ten minutes.
  </Card>
  <Card title="AI agents and MCP" icon="/assets/icons/terminal-window.svg" href="/mcp">
    An MCP server with the same permissions as the API, plus an off switch in AI control.
  </Card>
  <Card title="Templates" icon="/assets/icons/cloud-arrow-up.svg" href="/template-catalog">
    570+ one-click templates for databases, tools and AI stacks.
  </Card>
  <Card title="Observability" icon="/assets/icons/tree-structure.svg" href="/observability">
    Metrics, logs, monitors and alerts stored on your own nodes.
  </Card>
</CardGroup>

<CardGroup cols={2}>
  <Card title="Lower infrastructure cost" icon="/assets/icons/scales.svg" href="/lower-infrastructure-cost">
    Where cost drops: one control plane, commodity servers, sleep when idle.
  </Card>
  <Card title="Deploying apps" icon="/assets/icons/cloud-arrow-up.svg" href="/deploying-apps">
    Dockerfile, Compose and Railpack builds with readiness-gated cutover.
  </Card>
  <Card title="CLI reference" icon="/assets/icons/terminal-window.svg" href="/cli-reference">
    Every command, flag and exit code.
  </Card>
  <Card title="Architecture" icon="/assets/icons/tree-structure.svg" href="/architecture">
    The reconciler, the node agent and how state flows.
  </Card>
</CardGroup>

## What is in the product

- **Ship with git push.** Dockerfile, Compose and Railpack builds, previews, rollback. See [Deploying apps](/deploying-apps).
- **Templates.** 570+ templates in the catalog (574 catalog entries: 565 active, 9 deprecated). See [Template catalog](/template-catalog).
- **Automatic HTTPS and edge.** Ingress, firewall, rate limits. See [Domains and ingress](/domains-and-ingress) and [Firewall](/firewall).
- **Spin up nodes on any cloud.** Six providers. See [Node provisioning](/node-provisioning) and [Cloud integrations](/cloud-integrations).
- **Managed databases and backups.** 13 engines, restore drills. See [Managed databases](/managed-databases).
- **Built-in metrics, logs and alerts.** See [Monitors](/monitors) and [Status page](/status-page).
- **SSO, RBAC and audit log.** See [Identity and access](/identity-and-access).
- **CLI, API, Terraform and MCP.** See [CLI reference](/cli-reference), [API reference](/api-reference) and [Terraform provider](/terraform-provider).

All of it with counts and labels is on [All features](/features).

Every feature is labelled stable, beta or experimental in [Feature status](/feature-status).

## Identity and access

- Password, passkeys, TOTP, OAuth with 20 providers plus generic OpenID Connect, IAM policies, teams and a hash-chained audit log. See [Identity and access](/identity-and-access).
- SAML single sign-on and SCIM provisioning are beta and off by default. See [SAML and SCIM](/sso-saml).

## Works with

Levelrail drives Docker directly and plugs into the tools you already use.

<div className="flex flex-wrap items-center gap-2 not-prose"><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/docker/default.svg" alt="Docker" title="Docker" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/caddy/default.svg" alt="Caddy" title="Caddy" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/letsencrypt/default.svg" alt="Let's Encrypt" title="Let's Encrypt" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/wireguard/default.svg" alt="WireGuard" title="WireGuard" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/minio/default.svg" alt="MinIO" title="MinIO" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="26" height="26" /></span></div>
