---
title: "Levelrail documentation"
icon: "/assets/icons/house.svg"
sidebarTitle: "What is Levelrail"
description: Self-hosted deployment platform. Push to git, get a running app with TLS, logs, metrics and rollback. One Go binary, no Kubernetes.
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

## Works with

Levelrail drives Docker directly and plugs into the tools you already use.

<div className="flex flex-wrap items-center gap-2 not-prose"><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/docker/default.svg" alt="Docker" title="Docker" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/postgresql/default.svg" alt="PostgreSQL" title="PostgreSQL" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/redis/default.svg" alt="Redis" title="Redis" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/github/default.svg" alt="GitHub" title="GitHub" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/gitlab/default.svg" alt="GitLab" title="GitLab" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/caddy/default.svg" alt="Caddy" title="Caddy" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/letsencrypt/default.svg" alt="Let's Encrypt" title="Let's Encrypt" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/wireguard/default.svg" alt="WireGuard" title="WireGuard" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/minio/default.svg" alt="MinIO" title="MinIO" width="26" height="26" /></span><span className="inline-flex h-10 w-10 items-center justify-center rounded-lg bg-white p-2 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/grafana/default.svg" alt="Grafana" title="Grafana" width="26" height="26" /></span></div>
