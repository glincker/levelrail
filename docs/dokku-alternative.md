---
title: "Dokku alternative: git push deploys with a dashboard and multi-server"
description: "Levelrail is an open source Dokku alternative that adds a dashboard, deploy history with rollback, and built-in metrics and logs."
---

**When one server and git push stops being enough**

Dokku is a classic for a single host. Levelrail keeps the git push workflow and adds a dashboard, deploy history with one-click rollback, built-in observability and a way to add more servers.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## What you gain on top of git push

<CardGroup cols={3}>
  <Card title="A dashboard and history">
    See every deploy, its logs and its status in one place, and roll back to a pinned previous image with one click.
  </Card>
  <Card title="Metrics and logs built in">
    Node-local metrics at 15 second resolution and full-text log search, with deploy markers on the charts.
  </Card>
  <Card title="Room to grow">
    Add servers with a one-time join token and a small agent that dials out. Cordon, drain and explicit placement are supported.
  </Card>
</CardGroup>

## Dokku and Levelrail, side by side

Dokku has almost no idle footprint and is hard to beat for simplicity on one server. These are the design differences.

| | Dokku | Levelrail |
| --- | --- | --- |
| How it manages servers | Commands run on the host itself, driven over SSH or git push | A control plane with an agent per node that dials out over mTLS |
| Orchestration | A plugin-based scheduler over Docker on one host | Level-triggered reconciler with recorded status conditions |
| Ingress | nginx by default, other proxies via plugins | Caddy embedded in the control plane |
| Runs as | A set of scripts on the host, no separate web service | One Go binary with an embedded dashboard |
| Observability | Add your own stack | Metrics and log search in the core |

[Read the full comparison](/comparison)

## Getting started

<Steps>
  <Step title="Install on a Linux box">
    The install script handles Docker and the systemd service.
  </Step>
  <Step title="Connect a git repo">
    GitHub, GitLab, Bitbucket and Gitea webhooks trigger deploys.
  </Step>
  <Step title="Deploy and roll back">
    Traffic cuts over only after the readiness probe passes, and previous images stay pinned.
  </Step>
</Steps>

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Is there an importer for Dokku?">
    No. You deploy the same repositories and recreate environment variables and domains. The importer covers Coolify, Dokploy and CapRover.
  </Accordion>
  <Accordion title="Does Levelrail use buildpacks?">
    Levelrail builds Dockerfiles with BuildKit. Check the docs for the current state of automatic build detection.
  </Accordion>
  <Accordion title="Do I need more than one server?">
    No. Single node is the best tested path, and multi-server is there when you need it.
  </Accordion>
  <Accordion title="Is it free?">
    Yes. Apache 2.0, one edition.
  </Accordion>
</AccordionGroup>

## Add a dashboard to your deploys

Install it beside your current setup and compare.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)
