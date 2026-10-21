---
title: "Coolify alternative: a self-hosted deploy platform that stays light"
description: "Levelrail is an open source Coolify alternative: Docker Engine API instead of SSH, built-in metrics and logs, one-click rollback, read-only importer."
"og:image": "https://docs.levelrail.com/public/og/coolify-alternative.png"
"twitter:image": "https://docs.levelrail.com/public/og/coolify-alternative.png"
"og:url": "https://docs.levelrail.com/coolify-alternative"
---

**A lighter way to self-host what you deploy with Coolify**

Levelrail is an open source deployment platform for your own servers. Its agent talks to Docker's Engine API directly instead of SSHing in and shelling out, so the platform stays out of your apps' way.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

<img src="/assets/screenshots/apps-list-light.png" alt="Levelrail apps list showing every service, its status and its node" className="block dark:hidden" />
<img src="/assets/screenshots/apps-list.png" alt="Levelrail apps list showing every service, its status and its node" className="hidden dark:block" />

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

The importer reads your Coolify instance through its API and changes nothing there. It creates the apps and databases on your Levelrail server. Databases and volumes are created empty, so copy their data separately (see the migration guide). Run a dry run first.

```bash title="install, import, dry run"
curl -fsSL https://levelrail.com/install.sh | sudo sh
# on the server: sign in, then pass the Coolify API token from the environment
levelrail-cli login --device
export APP_IMPORT_SOURCE_TOKEN=...   # or pipe it in with --token-stdin
levelrail-cli import platform coolify --url https://coolify.example.com --dry-run
levelrail-cli import platform coolify --url https://coolify.example.com --only web,api
```

Re-running the import skips what it already created. For the full steps, including database copy and DNS cutover, see [Migrating from Coolify, Dokploy and CapRover](/migrating-from-coolify-dokploy-and-caprover).

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
    It covers the same job, deploying apps and databases to your own servers from a connected git provider, but it is a different design, not a fork. A read-only importer brings apps and databases across, and the [migration guide](/migrate-from-coolify) lists what maps and what does not.
  </Accordion>
  <Accordion title="Does Levelrail have Coolify's one-click service catalog?">
    Levelrail ships a catalog of about 360 one-click templates, a sample of which is deployed by its end-to-end tests. Coolify's catalog is in the same range. Check that the services you need are covered, in either catalog, before moving.
  </Accordion>
  <Accordion title="Can I run Levelrail on more than one server?">
    Yes. Extra servers join with a one-time token and run a small agent that dials out to the control plane, so managed servers never open an inbound port. Enrollment, cordon and drain are verified against real Docker daemons.
  </Accordion>
  <Accordion title="What does it cost?">
    Nothing. Levelrail is Apache 2.0 with one edition: OAuth and OIDC sign-in, audit logging, IAM and scheduled backups all ship in the same free binary. SAML SSO and SCIM provisioning are beta and off until you turn them on.
  </Accordion>
  <Accordion title="Is it production ready?">
    Levelrail is in active pre-release development, and single node is the best tested path. Run it next to your current setup first, which the importer's read-only design makes safe.
  </Accordion>
  <Accordion title="Does Levelrail support SSO, SAML and SCIM?">
    Levelrail has password, passkey and TOTP sign-in, OAuth sign-in with 20 providers and generic OpenID Connect, IAM policies, teams and a hash-chained audit log, all in the one free edition. SAML single sign-on and SCIM provisioning exist but are beta: they sit behind the `saml-scim` feature toggle, are off by default, and are tested against a test identity provider rather than a live Okta or Entra ID tenant. See [SAML and SCIM](/sso-saml). For what Coolify supports, see its own documentation.
  </Accordion>
</AccordionGroup>

## Try it next to your current setup

Import read-only, compare for a week, and keep what works. Open source, so you can read every line.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)
