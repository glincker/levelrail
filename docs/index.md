---
layout: home
title: Self-hosted deployment platform
description: Push to a git repo, get a running app with TLS, logs, metrics, and rollback. The agent talks to Docker's own Engine API directly, no SSH, no CLI shelling.

hero:
  name: Levelrail
  text: A self-hosted deployment platform
  tagline: Push to a git repo, get a running app with TLS, logs, metrics, and rollback. The agent talks to Docker's own Engine API directly, no SSH, no CLI shelling.
  actions:
    - theme: brand
      text: Get Started
      link: /getting-started
    - theme: alt
      text: View on GitHub
      link: https://github.com/glincker/levelrail
    - theme: alt
      text: Compare
      link: /comparison

features:
  - title: Zero-downtime deploys
    details: Rolling, recreate, or blue-green strategy, gated on real readiness and liveness probes, with rollback to pinned prior images always available.
  - title: Observability built in
    details: Node-local metrics at 15s resolution and full-text log search, no separate Grafana or Loki install. Deploy markers overlay directly on metric charts.
  - title: Eight managed database engines
    details: Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB, Dragonfly, and ClickHouse, with scheduled backups, restore, and automatic post-backup verification.
  - title: Multi-node from day one
    details: WireGuard mesh, internal DNS across nodes, cordon and drain, no inbound ports required on any managed server.
  - title: Know what needs attention
    details: A Status page and an attention CLI command list failing apps, offline nodes, expiring certificates, and doctor findings, with a disk pressure banner and stalled certificate renewal detection.
  - title: Resource-scoped IAM
    details: AWS-IAM-shaped Allow/Deny policies scoped to a specific app or database, with a full audit log and CSV export, in the free Apache 2.0 core.
  - title: AI-ready API
    details: 144 MCP tools (beta) backed by the same HTTP API the dashboard runs on, so AI tools can list apps, read logs, and diagnose a crashloop directly.
---

<div class="vp-doc landing-body">

<section class="landing-section landing-section--quickstart">

## Quickstart

<div class="quickstart-shell">

::: code-group

```bash [Install]
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

```yaml [app.yaml]
version: 1
services:
  web:
    build:
      type: dockerfile
    port: 8080
```

:::

Then deploy it:

```bash
levelrail-cli apps create --name your-app --file app.yaml --repo https://github.com/your-org/your-app
```

</div>

That is a running app with HTTPS, logs, and rollback. The full walkthrough, including the setup wizard and database attachment, is in [Getting started](/getting-started).

</section>

<section class="landing-section landing-section--screenshots">

## See it running

<div class="screenshot-grid">
  <div class="screenshot-frame"><img src="/assets/screenshots/apps-list.png" alt="Levelrail apps list showing all services across nodes at a glance" loading="lazy" width="1280" height="800"></div>
  <div class="screenshot-frame"><img src="/assets/screenshots/deploy-history.png" alt="Levelrail deploy history view with one-click rollback" loading="lazy" width="1280" height="800"></div>
  <div class="screenshot-frame"><img src="/assets/screenshots/logs.png" alt="Levelrail live log viewer with full-text search" loading="lazy" width="1280" height="800"></div>
  <div class="screenshot-frame"><img src="/assets/screenshots/nodes.png" alt="Levelrail nodes list showing node health and placement" loading="lazy" width="1280" height="700"></div>
</div>

</section>

</div>

