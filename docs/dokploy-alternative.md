---
title: "Dokploy alternative: self-hosted deploys without Docker Swarm"
description: "Levelrail is an open source Dokploy alternative that skips Docker Swarm: an agent on each node and a reconciler over the Docker Engine API."
---

**Self-hosted deploys without betting on Docker Swarm**

Levelrail runs apps on plain Docker with its own reconciler and a small agent per node. No Swarm and no overlay network to debug.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

![Levelrail apps list showing every service, its status and its node](/assets/screenshots/apps-list.png)

## What is different under the hood

<CardGroup cols={3}>
  <Card title="A reconciler, not Swarm services">
    Desired state lives in the database and a level-triggered loop converges each node toward it, recording a status condition with a reason after every pass.
  </Card>
  <Card title="Agent per node, nothing inbound">
    Each extra server runs a small agent that dials out to the control plane with mutual TLS. Cordon and drain relocate containers between real Docker daemons.
  </Card>
  <Card title="Observability is core">
    Metrics, full-text logs, crashloop detection with the last log lines, and alerts to 18 notification channel kinds all ship in the box, with no extra stack to install.
  </Card>
</CardGroup>

## Bring your apps with you

The importer reads your existing instance through its own HTTP API and never changes anything there. Run it with --dry-run first to see exactly what would be created.

```bash title="install, import, verify"
curl -fsSL https://levelrail.com/install.sh | sudo sh
# control plane running as a systemd service
levelrail-cli import platform dokploy --url https://dokploy.example.com --dry-run
# dry run: report only, nothing is written here or changed there
levelrail-cli import platform dokploy --url https://dokploy.example.com --only web,api
# apps and databases created, ready to deploy
```

## Dokploy and Levelrail, side by side

Dokploy is a polished project built on a mature mechanism. These are the design differences.

| | Dokploy | Levelrail |
| --- | --- | --- |
| Multi-node | Docker Swarm | Agent per node over gRPC with mTLS |
| Orchestration | Swarm services and Docker Compose | Level-triggered reconciler over the Docker Engine API |
| Ingress | Traefik | Caddy embedded in the control plane |
| Runs as | A web application with its own database | One Go binary with embedded SQLite |
| Observability | Monitoring features in the dashboard | Node-local metrics and log search with deploy markers, federated across agents |

[Read the full comparison](/comparison)

## Evaluate it safely

<Steps>
  <Step title="Install on a spare box">
    Run the install script on any Linux server with outbound access.
  </Step>
  <Step title="Import read-only">
    The importer only reads from Dokploy through its API. Use --dry-run to see the report first.
  </Step>
  <Step title="Deploy and compare">
    Run both for a while. Rollback, readiness gating and the built-in charts are easy to judge side by side.
  </Step>
</Steps>

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Does Levelrail use Docker Swarm?">
    No. It runs plain Docker containers on each node, driven through the Docker Engine API by a reconciler. Placement across nodes is explicit, and cordon and drain are supported.
  </Accordion>
  <Accordion title="How do I migrate from Dokploy?">
    Use the platform importer, levelrail-cli import platform dokploy with your Dokploy URL. It is read-only against the source and supports --dry-run and --only.
  </Accordion>
  <Accordion title="Is Levelrail free?">
    Yes, Apache 2.0 with a single edition and no feature gated behind a plan.
  </Accordion>
  <Accordion title="How mature is it?">
    It is pre-release and moving fast. Single node is the most tested path, and multi-node enrollment, cordon and drain have been verified against real Docker daemons.
  </Accordion>
</AccordionGroup>

## See how a Swarm-free setup feels

Install it on a spare server and import read-only. If it is not for you, nothing was changed.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)
