---
title: "Levelrail demo: install, deploy and roll back in minutes"
description: "See Levelrail in action: install script, git push deploys, live logs, metrics with deploy markers, multi-node and one-click rollback."
"og:url": "https://docs.levelrail.com/demo"
---

**See a deploy from install to rollback**

A walkthrough of the dashboard and the CLI, using real screenshots. No sign-up, nothing to host on our side: try the same steps on your own server.

[Run it yourself](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

<img src="/assets/screenshots/dashboard-home-light.png" alt="Levelrail dashboard home with apps, nodes and recent deploys" className="block dark:hidden" />
<img src="/assets/screenshots/dashboard-home.png" alt="Levelrail dashboard home with apps, nodes and recent deploys" className="hidden dark:block" />

## Install and first deploy

The install script checks the host, installs Docker if it is missing and starts the control plane as a systemd service. Then you deploy.

```bash title="install, deploy"
curl -fsSL https://levelrail.com/install.sh | sudo sh
# checking host: docker, systemd, ports 80/443/8080 free
# installing control plane as a systemd service
# dashboard ready, setup token printed above
levelrail-cli deploy myapp --image registry.example.com/acme/myapp:latest
# readiness probe passed, cutting traffic
# myapp is live over HTTPS
```

## What to try first

<Steps>
  <Step title="Install and open the dashboard">
    Run the install script, then sign in with the setup token it prints.
  </Step>
  <Step title="Deploy something small">
    Connect a repo or point at an image and watch the build and the readiness gate.
  </Step>
  <Step title="Break it on purpose">
    Deploy a failing version, see the crashloop detection with the last log lines, then roll back.
  </Step>
</Steps>

## A tour of the dashboard

<Frame caption="Every app across every node, with status at a glance.">
  <img src="/assets/screenshots/apps-list-light.png" alt="Levelrail apps list" className="block dark:hidden" />
  <img src="/assets/screenshots/apps-list.png" alt="Levelrail apps list" className="hidden dark:block" />
</Frame>

<Frame caption="Deploy history with one-click rollback to a pinned image.">
  <img src="/assets/screenshots/deploy-history-light.png" alt="Levelrail deploy history" className="block dark:hidden" />
  <img src="/assets/screenshots/deploy-history.png" alt="Levelrail deploy history" className="hidden dark:block" />
</Frame>

<Frame caption="Live log tail with full-text search.">
  <img src="/assets/screenshots/logs-light.png" alt="Levelrail live log viewer" className="block dark:hidden" />
  <img src="/assets/screenshots/logs.png" alt="Levelrail live log viewer" className="hidden dark:block" />
</Frame>

<Frame caption="Metrics with deploy markers overlaid on the charts.">
  <img src="/assets/screenshots/app-overview-light.png" alt="Levelrail app overview with metrics" className="block dark:hidden" />
  <img src="/assets/screenshots/app-overview.png" alt="Levelrail app overview with metrics" className="hidden dark:block" />
</Frame>

<Frame caption="Nodes, their health and what is placed on them.">
  <img src="/assets/screenshots/nodes-light.png" alt="Levelrail nodes list" className="block dark:hidden" />
  <img src="/assets/screenshots/nodes.png" alt="Levelrail nodes list" className="hidden dark:block" />
</Frame>

<Frame caption="Managed databases with backups and verification.">
  <img src="/assets/screenshots/database-overview-light.png" alt="Levelrail database overview" className="block dark:hidden" />
  <img src="/assets/screenshots/database-overview.png" alt="Levelrail database overview" className="hidden dark:block" />
</Frame>

## Terminal recordings

<Frame caption="Install and first deploy, from a fresh server to a live app.">
  <img src="/assets/demos/install-deploy.gif" alt="Terminal recording: run the install script, then deploy an app that goes live" />
</Frame>

<Frame caption="Deploy with a readiness gate, then roll back in one command.">
  <img src="/assets/demos/deploy-rollback.gif" alt="Terminal recording: deploy with readiness gating, then roll back to the previous image" />
</Frame>

<Frame caption="Create a managed database and query it without leaving the terminal.">
  <img src="/assets/demos/db-create.gif" alt="Terminal recording: create a managed Postgres database, inspect it and run a read-only query" />
</Frame>

<Frame caption="Browse the template catalog and deploy a service.">
  <img src="/assets/demos/templates.gif" alt="Terminal recording: browse the template catalog and deploy one" />
</Frame>

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Is there a hosted demo?">
    No. Levelrail is self-hosted, so the quickest demo is the install script on any Linux box. The screenshots here are from a real dashboard.
  </Accordion>
  <Accordion title="How long does the install take?">
    The install script handles the host check, Docker and the systemd service in one run. Most of the time is Docker pulling images on a fresh server.
  </Accordion>
  <Accordion title="Can I try it without touching my current setup?">
    Yes. Install on a spare server. If you use the importer, it is read-only against your existing platform.
  </Accordion>
</AccordionGroup>

## Run the demo on your own server

Ten minutes, one small VPS, nothing to sign up for.

[Run it yourself](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)
