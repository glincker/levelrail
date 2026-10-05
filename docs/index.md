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
    details: 'Blue-green (the default) and rolling deploys move traffic only after the new container passes its readiness probe, and prior images stay pinned so rollback is always one click away. Recreate is there when a short gap is fine.<span class="feature-proof feature-proof--chips"><span class="feature-chip">rolling</span><span class="feature-chip">recreate</span><span class="feature-chip">blue-green</span></span>'
  - title: Observability built in
    details: 'Node-local metrics at 15s resolution and full-text log search, with 15 days of retention by default and no separate Grafana or Loki install. Deploy markers overlay directly on metric charts.<span class="feature-proof feature-proof--stat"><span class="feature-stat-value">15s</span><span class="feature-stat-label">metric resolution</span></span>'
  - title: Eight managed database engines
    details: 'Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB, Dragonfly, and ClickHouse, with scheduled backups, restore, and automatic post-backup verification.<span class="feature-proof feature-proof--chips feature-proof--chips-wrap"><span class="feature-chip feature-chip--mono">postgres</span><span class="feature-chip feature-chip--mono">redis</span><span class="feature-chip feature-chip--mono">mysql</span><span class="feature-chip feature-chip--mono">mongodb</span><span class="feature-chip feature-chip--mono">mariadb</span><span class="feature-chip feature-chip--mono">keydb</span><span class="feature-chip feature-chip--mono">dragonfly</span><span class="feature-chip feature-chip--mono">clickhouse</span></span>'
  - title: Multi-node, no inbound ports
    icon: '<svg width="64" height="36" viewBox="0 0 64 36" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true"><line x1="8" y1="9" x2="32" y2="28" stroke="currentColor" stroke-width="1.5" opacity="0.45"/><line x1="32" y1="28" x2="56" y2="9" stroke="currentColor" stroke-width="1.5" opacity="0.45"/><line x1="8" y1="9" x2="56" y2="9" stroke="currentColor" stroke-width="1.5" opacity="0.3"/><line x1="32" y1="28" x2="32" y2="7" stroke="currentColor" stroke-width="1.5" opacity="0.3" stroke-dasharray="2 3"/><circle cx="8" cy="9" r="3.5" fill="currentColor"/><circle cx="56" cy="9" r="3.5" fill="currentColor"/><circle cx="32" cy="28" r="3.5" fill="currentColor"/><circle cx="32" cy="7" r="2.5" fill="currentColor" opacity="0.55"/></svg>'
    details: Add servers with a one-time join token. Every agent dials out over mTLS, so managed servers open no inbound ports, and you can cordon, drain, and pin apps to nodes. The WireGuard mesh is beta.
  - title: Know what needs attention
    details: 'A Status page and the `attention` CLI command list failing apps, offline nodes, expiring certificates, and doctor findings, with a disk pressure banner and stalled certificate renewal detection. Alerts reach you through 18 notification channel kinds, including Slack, Discord, email, Telegram, PagerDuty, and ntfy.<span class="feature-proof feature-proof--status"><span class="feature-status"><span class="feature-dot feature-dot--bad"></span>app failing</span><span class="feature-status"><span class="feature-dot feature-dot--warn"></span>cert expiring</span><span class="feature-status"><span class="feature-dot feature-dot--off"></span>node offline</span></span>'
  - title: Resource-scoped IAM
    details: 'AWS-IAM-shaped Allow/Deny policies scoped to a specific app or database, with a full audit log and CSV export, in the free Apache 2.0 core.<span class="feature-proof feature-proof--code"><code class="feature-code-line">allow: app:web:deploy</code></span>'
  - title: 311 one-click templates
    details: 'Self-hosted services such as n8n, Gitea, Uptime Kuma, and Vaultwarden, each a Compose file that Levelrail deploys and manages like any other app.<span class="feature-proof feature-proof--chips"><span class="feature-chip feature-chip--mono">n8n</span><span class="feature-chip feature-chip--mono">gitea</span><span class="feature-chip feature-chip--mono">uptime-kuma</span><span class="feature-chip feature-chip--mono">vaultwarden</span></span>'
  - title: AI-ready API
    details: '156 MCP tools (beta) backed by the same HTTP API the dashboard runs on, so AI tools can list apps, read logs, and diagnose a failed deploy directly. The count by toolset is in the <a href="/mcp-tool-surface">MCP tool surface</a>.<span class="feature-proof feature-proof--code"><code class="feature-code-line">mcp.call("get_app_logs", name="web")</code></span>'
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

The install script checks the host, installs Docker if it is missing, and starts the control plane as a systemd service. `levelrail-cli deploy` points an app at an image and cuts traffic over only once the new container's readiness probe passes. To build from a git repository instead, connect it from the dashboard or run `levelrail-cli import <repo-url> --deploy`. The full walkthrough, including the setup wizard, is in [Getting started](/getting-started).

</section>

<section class="landing-section landing-section--steps">

## How it works

Four steps, the same ones the reconciler itself runs on every deploy. Click a step to see what it actually does.

<HowItWorksFlow />

</section>

<section class="landing-section landing-section--features-tabs">

## Explore the platform

<FeatureTabsSection />

</section>

<section class="landing-section landing-section--compare">

## How it compares

Many self-hosted platforms in this category manage servers by SSHing in and running `docker` CLI commands, then parsing the text output. That tends to force polling loops, which is a common source of flakiness and idle CPU use. Levelrail takes a different route.

<div class="compare-table">

<div class="compare-table__row compare-table__row--head" role="presentation">
<span class="compare-table__cell compare-table__cell--label"></span>
<span class="compare-table__cell compare-table__cell--before">SSH + shell out</span>
<span class="compare-table__cell compare-table__cell--after">Levelrail</span>
</div>

<div class="compare-table__row">
<span class="compare-table__cell compare-table__cell--label">Server management</span>
<span class="compare-table__cell compare-table__cell--before"><PhX class="compare-table__icon compare-table__icon--before" weight="bold" /><span class="compare-table__cell-text">SSHes into every node and runs <code>docker</code> CLI commands, then parses the text output.</span></span>
<span class="compare-table__cell compare-table__cell--after"><PhCheck class="compare-table__icon compare-table__icon--after" weight="bold" /><span class="compare-table__cell-text">The agent dials out over mTLS and talks to the Docker Engine API directly. Nothing shells out to the <code>docker</code> CLI.</span></span>
</div>

<div class="compare-table__row">
<span class="compare-table__cell compare-table__cell--label">Orchestration</span>
<span class="compare-table__cell compare-table__cell--before"><PhX class="compare-table__icon compare-table__icon--before" weight="bold" /><span class="compare-table__cell-text">Polling loops that often leave no recorded reason for why a resource is in its current state.</span></span>
<span class="compare-table__cell compare-table__cell--after"><PhCheck class="compare-table__icon compare-table__icon--after" weight="bold" /><span class="compare-table__cell-text">A level-triggered reconciler diffs desired against observed state and writes a status condition with a reason after every pass.</span></span>
</div>

<div class="compare-table__row">
<span class="compare-table__cell compare-table__cell--label">Observability</span>
<span class="compare-table__cell compare-table__cell--before"><PhX class="compare-table__icon compare-table__icon--before" weight="bold" /><span class="compare-table__cell-text">Often an add-on: install Grafana or Loki yourself, then wire them up for metrics and logs.</span></span>
<span class="compare-table__cell compare-table__cell--after"><PhCheck class="compare-table__icon compare-table__icon--after" weight="bold" /><span class="compare-table__cell-text">Node-local metrics at 15 second resolution and full-text log search are built in, no separate install.</span></span>
</div>

<div class="compare-table__row">
<span class="compare-table__cell compare-table__cell--label">Footprint</span>
<span class="compare-table__cell compare-table__cell--before"><PhX class="compare-table__icon compare-table__icon--before" weight="bold" /><span class="compare-table__cell-text">Often a stack of separate services: reverse proxy, metrics store, log store, dashboard.</span></span>
<span class="compare-table__cell compare-table__cell--after"><PhCheck class="compare-table__icon compare-table__icon--after" weight="bold" /><span class="compare-table__cell-text">SQLite in WAL mode, an embedded Caddy ingress, and an embedded dashboard: one control plane binary on one node. See <a href="/performance">measured idle footprint</a>.</span></span>
</div>

</div>

<a class="compare-cta" href="/comparison">See the full comparison against Coolify, Dokploy, CapRover, Dokku, and Kamal</a>

</section>

<section class="landing-section landing-section--statement">

<div class="statement-block">

<p class="statement-text">Not a Kubernetes competitor.<br>Not a Vercel competitor.</p>

<p class="statement-context">It is built for running 3 to 50 services on 1 to 10 machines without learning Kubernetes.</p>

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

<section class="landing-section landing-section--releases">

## Latest releases

<LatestReleasesSection />

</section>

<section class="landing-section landing-section--faq">

## Frequently asked questions

<FaqSection />

</section>

</div>

