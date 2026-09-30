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

<script setup lang="ts">
import TerminalDemo from './.vitepress/theme/TerminalDemo.vue'
</script>

<div class="vp-doc landing-body">

<section class="landing-section landing-section--quickstart">

## Quickstart

<div class="quickstart-shell">
  <TerminalDemo />
</div>

That install script checks the host, installs Docker if it's missing, and starts the control plane as a systemd service. `deploy` builds from `app.yaml` and only cuts traffic to the new container once its readiness probe passes. The full walkthrough, including the setup wizard and database attachment, is in [Getting started](/getting-started).

</section>

<section class="landing-section landing-section--steps">

## How it works

<div class="steps-grid">

<div class="step-card">

<span class="step-card__index">01</span>

**Push to your git repo**

GitHub, GitLab, or Bitbucket webhooks trigger a deploy on every push, with preview environments per pull request.

</div>

<div class="step-card">

<span class="step-card__index">02</span>

**Build**

A Dockerfile, a Compose file, or Railpack auto-detection builds through BuildKit, with remote cache and live log streaming.

</div>

<div class="step-card">

<span class="step-card__index">03</span>

**Live app**

TLS from the embedded Caddy ingress, node-local metrics and logs, and rollback to a pinned prior image, with no extra setup.

</div>

</div>

</section>

<section class="landing-section landing-section--compare">

## How it compares

<div class="compare-grid">

<div class="compare-card">

**Server management**

An agent dials out over mTLS and talks to the Docker Engine API directly. Nothing shells out to the `docker` CLI.

</div>

<div class="compare-card">

**Orchestration**

A level-triggered reconciler diffs desired against observed state and writes a status condition with a reason after every pass.

</div>

<div class="compare-card">

**Observability**

Node-local metrics at 15 second resolution and full-text log search are built in, no separate Grafana or Loki install.

</div>

<div class="compare-card">

**Multi-node networking**

A WireGuard mesh and internal DNS connect nodes, with no inbound ports required on any managed server.

</div>

<div class="compare-card">

**Rollback**

Prior images are pinned, so garbage collection cannot remove a rollback target.

</div>

<div class="compare-card">

**Footprint**

SQLite in WAL mode, an embedded Caddy, and an embedded dashboard: one binary on one node.

</div>

</div>

<a class="compare-cta" href="/comparison">See the full comparison against Coolify, Dokploy, CapRover, Dokku, and Kamal</a>

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

