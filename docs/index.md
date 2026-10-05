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
      text: Quickstart
      link: /getting-started
    - theme: alt
      text: Live demo
      link: /demo
    - theme: alt
      text: Compare
      link: /comparison
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

<p class="compare-more">Switching? See the <a href="/coolify-alternative">Coolify</a>, <a href="/dokploy-alternative">Dokploy</a>, <a href="/vercel-alternative">Vercel</a>, <a href="/heroku-alternative">Heroku</a> and <a href="/railway-alternative">Railway</a> guides, or the <a href="/demo">demo</a> and <a href="/pricing">pricing</a>.</p>

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

