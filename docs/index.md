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
import HowItWorksFlow from './.vitepress/theme/HowItWorksFlow.vue'
import { PhCheck, PhX } from '@phosphor-icons/vue'
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

Four steps, the same ones the reconciler itself runs on every deploy. Click a step to see what it actually does.

<HowItWorksFlow />

</section>

<section class="landing-section landing-section--compare">

## How it compares

Most self-hosted PaaS tools in this category drive remote servers by SSHing in and shelling out `docker` CLI commands, then parsing text output. That's the source of most of the flakiness and the idle CPU burn, because it forces polling loops. Levelrail doesn't do that.

<div class="compare-table">

<div class="compare-table__row compare-table__row--head" role="presentation">
<span class="compare-table__cell compare-table__cell--label"></span>
<span class="compare-table__cell compare-table__cell--before">SSH + shell out</span>
<span class="compare-table__cell compare-table__cell--after">Levelrail</span>
</div>

<div class="compare-table__row">
<span class="compare-table__cell compare-table__cell--label">Server management</span>
<span class="compare-table__cell compare-table__cell--before"><PhX class="compare-table__icon compare-table__icon--before" weight="bold" /><span class="compare-table__cell-text">SSHes into every node and shells out <code>docker</code> CLI commands, then parses text output.</span></span>
<span class="compare-table__cell compare-table__cell--after"><PhCheck class="compare-table__icon compare-table__icon--after" weight="bold" /><span class="compare-table__cell-text">The agent dials out over mTLS and talks to the Docker Engine API directly. Nothing shells out to the <code>docker</code> CLI.</span></span>
</div>

<div class="compare-table__row">
<span class="compare-table__cell compare-table__cell--label">Orchestration</span>
<span class="compare-table__cell compare-table__cell--before"><PhX class="compare-table__icon compare-table__icon--before" weight="bold" /><span class="compare-table__cell-text">Ad hoc polling loops, with no recorded reason for why a resource is in its current state.</span></span>
<span class="compare-table__cell compare-table__cell--after"><PhCheck class="compare-table__icon compare-table__icon--after" weight="bold" /><span class="compare-table__cell-text">A level-triggered reconciler diffs desired against observed state and writes a status condition with a reason after every pass.</span></span>
</div>

<div class="compare-table__row">
<span class="compare-table__cell compare-table__cell--label">Observability</span>
<span class="compare-table__cell compare-table__cell--before"><PhX class="compare-table__icon compare-table__icon--before" weight="bold" /><span class="compare-table__cell-text">Bolted on: install Grafana or Loki yourself, then wire them up to get metrics and logs.</span></span>
<span class="compare-table__cell compare-table__cell--after"><PhCheck class="compare-table__icon compare-table__icon--after" weight="bold" /><span class="compare-table__cell-text">Node-local metrics at 15 second resolution and full-text log search are built in, no separate install.</span></span>
</div>

<div class="compare-table__row">
<span class="compare-table__cell compare-table__cell--label">Footprint</span>
<span class="compare-table__cell compare-table__cell--before"><PhX class="compare-table__icon compare-table__icon--before" weight="bold" /><span class="compare-table__cell-text">A stack of separate services: reverse proxy, metrics store, log store, dashboard.</span></span>
<span class="compare-table__cell compare-table__cell--after"><PhCheck class="compare-table__icon compare-table__icon--after" weight="bold" /><span class="compare-table__cell-text">SQLite in WAL mode, an embedded Caddy ingress, and an embedded dashboard: one binary on one node.</span></span>
</div>

</div>

<a class="compare-cta" href="/comparison">See the full comparison against Coolify, Dokploy, CapRover, Dokku, and Kamal</a>

</section>

<section class="landing-section landing-section--statement">

<div class="statement-block">

<p class="statement-text">Not a Kubernetes competitor.<br>Not a Vercel competitor.</p>

<p class="statement-context">The target user runs between 3 and 50 services on between 1 and 10 machines, and doesn't want to learn Kubernetes.</p>

</div>

</section>

<section class="landing-section landing-section--screenshots">

## See it running

<div class="screenshot-grid">
  <div class="screenshot-frame">
    <div class="screenshot-frame__chrome" aria-hidden="true"><span class="screenshot-frame__dot screenshot-frame__dot--red"></span><span class="screenshot-frame__dot screenshot-frame__dot--yellow"></span><span class="screenshot-frame__dot screenshot-frame__dot--green"></span><span class="screenshot-frame__url">levelrail.local/apps</span></div>
    <img src="/assets/screenshots/apps-list.png" alt="Levelrail apps list showing all services across nodes at a glance" loading="lazy" width="1280" height="800">
  </div>
  <div class="screenshot-frame">
    <div class="screenshot-frame__chrome" aria-hidden="true"><span class="screenshot-frame__dot screenshot-frame__dot--red"></span><span class="screenshot-frame__dot screenshot-frame__dot--yellow"></span><span class="screenshot-frame__dot screenshot-frame__dot--green"></span><span class="screenshot-frame__url">levelrail.local/apps/web/deploys</span></div>
    <img src="/assets/screenshots/deploy-history.png" alt="Levelrail deploy history view with one-click rollback" loading="lazy" width="1280" height="800">
  </div>
  <div class="screenshot-frame">
    <div class="screenshot-frame__chrome" aria-hidden="true"><span class="screenshot-frame__dot screenshot-frame__dot--red"></span><span class="screenshot-frame__dot screenshot-frame__dot--yellow"></span><span class="screenshot-frame__dot screenshot-frame__dot--green"></span><span class="screenshot-frame__url">levelrail.local/apps/web/logs</span></div>
    <img src="/assets/screenshots/logs.png" alt="Levelrail live log viewer with full-text search" loading="lazy" width="1280" height="800">
  </div>
  <div class="screenshot-frame">
    <div class="screenshot-frame__chrome" aria-hidden="true"><span class="screenshot-frame__dot screenshot-frame__dot--red"></span><span class="screenshot-frame__dot screenshot-frame__dot--yellow"></span><span class="screenshot-frame__dot screenshot-frame__dot--green"></span><span class="screenshot-frame__url">levelrail.local/nodes</span></div>
    <img src="/assets/screenshots/nodes.png" alt="Levelrail nodes list showing node health and placement" loading="lazy" width="1280" height="700">
  </div>
</div>

</section>

</div>

