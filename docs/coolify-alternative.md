---
title: "Coolify alternative: a self-hosted deploy platform that stays light"
description: "Levelrail is an open source Coolify alternative: Docker Engine API instead of SSH, built-in metrics and logs, one-click rollback, read-only importer."
"og:image": "https://docs.levelrail.com/public/og/coolify-alternative.png"
"twitter:image": "https://docs.levelrail.com/public/og/coolify-alternative.png"
---

**A lighter way to self-host what you deploy with Coolify**

Levelrail is an open source deployment platform for your own servers. Its agent talks to Docker's Engine API directly instead of SSHing in and shelling out, so the platform stays out of your apps' way.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

![Levelrail apps list showing every service, its status and its node](/assets/screenshots/apps-list.png)

## Why people look for a Coolify alternative

<CardGroup cols={3}>
  <Card title="No SSH, no CLI parsing">
    A node agent dials out to the control plane over mutual TLS and uses the Docker Engine API. Nothing shells out to the docker CLI, and managed servers need no inbound port.
  </Card>
  <Card title="Built-in metrics and logs">
    Node-local metrics at 15 second resolution and full-text log search ship in the core. Deploy markers sit on the charts, so which deploy caused a spike is a visual answer.
  </Card>
  <Card title="Rollback that survives cleanup">
    Previous images stay pinned so garbage collection can never remove your rollback target. Traffic only cuts over once the new container passes a real readiness probe.
  </Card>
</CardGroup>

## Bring your apps with you

The importer reads your existing instance through its own HTTP API and never changes anything there. Run it with --dry-run first to see exactly what would be created.

```bash title="install, import, verify"
curl -fsSL https://levelrail.com/install.sh | sudo sh
# control plane running as a systemd service
levelrail-cli import platform coolify --url https://coolify.example.com --dry-run
# dry run: report only, nothing is written here or changed there
levelrail-cli import platform coolify --url https://coolify.example.com --only web,api
# apps and databases created, ready to deploy
```

## Coolify and Levelrail, side by side

Architecture and design choices, not a ranking. Coolify is a mature project with a wide template catalog.

| | Coolify | Levelrail |
| --- | --- | --- |
| Server management | SSH from the control plane to each server | Agent dials out over gRPC with mTLS, talks to the Docker Engine API |
| Orchestration | Docker and Docker Compose per resource | Level-triggered reconciler: desired state in the database, converged continuously |
| Ingress | Traefik (Caddy also offered) | Caddy embedded in the control plane |
| Runs as | A web application with its own database | One control plane binary, SQLite in WAL mode, one agent binary per extra node |
| License | Open source | Apache 2.0, one edition, no paid tier |

[Read the full comparison](/comparison)

## Move over in four steps

<Steps>
  <Step title="Install on a Linux box">
    One script checks the host, installs Docker if missing, and starts the control plane as a systemd service.
  </Step>
  <Step title="Dry-run the importer">
    Point it at your Coolify URL with --dry-run. You get a report of what would be created. Nothing changes on either side.
  </Step>
  <Step title="Import and deploy">
    Import all apps and databases or a subset with --only, then deploy and watch the readiness probe gate the cutover.
  </Step>
  <Step title="Cut DNS when you are ready">
    Keep Coolify running until the new setup has earned your trust, then switch your domains.
  </Step>
</Steps>

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Is Levelrail a drop-in replacement for Coolify?">
    It covers the same job, deploying apps and databases to your own servers from a git push, but it is a different design, not a fork. A read-only importer brings apps and databases across, and the migration guide lists what maps and what does not.
  </Accordion>
  <Accordion title="Does Levelrail have Coolify's one-click service catalog?">
    Levelrail ships a catalog of 311 one-click templates, a sample of which is deployed by its end-to-end tests. Check that the services you need are covered, in either catalog, before moving.
  </Accordion>
  <Accordion title="Can I run Levelrail on more than one server?">
    Yes. Extra servers join with a one-time token and run a small agent that dials out to the control plane, so managed servers never open an inbound port. Enrollment, cordon and drain are verified against real Docker daemons.
  </Accordion>
  <Accordion title="What does it cost?">
    Nothing. Levelrail is Apache 2.0 with one edition: SSO, audit logging, IAM and scheduled backups all ship in the same free binary.
  </Accordion>
  <Accordion title="Is it production ready?">
    Levelrail is in active pre-release development, and single node is the best tested path. Run it next to your current setup first, which the importer's read-only design makes safe.
  </Accordion>
</AccordionGroup>

## Try it next to your current setup

Import read-only, compare for a week, and keep what works. Open source, so you can read every line.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)
