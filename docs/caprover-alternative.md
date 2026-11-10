---
title: "CapRover alternative: self-hosted deploys without Docker Swarm"
description: "Levelrail is an open source CapRover alternative: plain Docker instead of Swarm, an agent per node, built-in metrics and logs, one-click rollback."
"og:url": "https://docs.levelrail.com/caprover-alternative"
---

**Keep the simple CapRover workflow, drop the Swarm dependency**

Levelrail runs apps on plain Docker containers with its own reconciler and a small agent per extra server. Push-to-deploy from your git host, automatic HTTPS, rollback and an importer for your CapRover apps.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

## Where Levelrail differs

<CardGroup cols={3}>
  <Card title="No Swarm, plain Docker">
    Apps run as ordinary containers. A level-triggered reconciler compares desired and observed state and converges, recording a reason after every pass.
  </Card>
  <Card title="Agent per node">
    Extra servers run a small agent that dials out to the control plane with mutual TLS, so managed servers need no inbound port.
  </Card>
  <Card title="Observability included">
    Node-local metrics, full-text log search, crashloop detection and alerts to eighteen channels ship in the core, with deploy markers on the charts.
  </Card>
</CardGroup>

## Bring your apps with you

The importer reads your existing instance through its own HTTP API and never changes anything there. Run it with --dry-run first.

```bash title="install, import, verify"
curl -fsSL https://levelrail.com/install.sh | sudo sh
# control plane running as a systemd service
levelrail-cli import platform caprover --url https://captain.example.com --dry-run
# dry run: report only, nothing is written here or changed there
levelrail-cli import platform caprover --url https://captain.example.com --only web,api
# apps and databases created, ready to deploy
```

## CapRover and Levelrail, side by side

CapRover is a long-standing project with a large install base and a one-click app store. These are the design differences.

| | CapRover | Levelrail |
| --- | --- | --- |
| Server management | Docker Swarm | Agent per node over gRPC with mutual TLS, Docker Engine API |
| Orchestration | Swarm services | Level-triggered reconciler over plain Docker |
| Ingress | nginx | Caddy embedded in the control plane |
| Runs as | A web application running as a Swarm service | One Go binary with embedded SQLite |
| License | Open source | Apache 2.0, one edition |

[Read the full comparison](/comparison)

## Where each familiar feature lives

Every command below exists on the current release branch. Run `levelrail-cli <command> --help` for the full flag list.

| If you used this in CapRover | In Levelrail | Command |
| --- | --- | --- |
| Deploy an image | App deploy | `levelrail-cli apps deploy <name> --image IMAGE` |
| Deploy a docker-compose file | Compose deploy | `levelrail-cli compose deploy <app> -f compose.yaml` |
| One-click apps | Template catalog | `levelrail-cli templates install <id>` |
| Previous deploys and rollback | Release history | `levelrail-cli apps rollback <name>` |
| Instance count | Replicas and resource limits | `levelrail-cli apps scale <name> --replicas N` |
| Custom domains and HTTPS | Domains with automatic certificates | `levelrail-cli apps domains add <name> <domain>` |
| Environment variables | App env with encrypted secrets | `levelrail-cli apps env import <name> --file .env` |
| Persistent directories | App volumes | `levelrail-cli apps volumes attach <name> --name NAME --path PATH` |
| App logs | Log viewer and search | `levelrail-cli apps logs <name>` |
| Shell into a container | Exec and console | `levelrail-cli apps console <name>` |
| Backups | Backup targets and schedules | `levelrail-cli apps backups schedule set <app> <service> --target ID --cron EXPR` |
| Cluster nodes | Agent per node | `levelrail-cli nodes list` |
| Git push-to-deploy | Repository source with webhooks | `levelrail-cli apps webhook-deliveries list <app-name>` |

## Not there yet

- **Definition files.** CapRover reads a `captain-definition` file from the repository. Levelrail has no equivalent file format yet; describe the app in the dashboard, with `apps deploy-spec`, or with platform-as-code. A definition-file format is planned.
- **Template import.** You cannot paste a CapRover one-click app definition into Levelrail yet. Pick the matching entry from the template catalog or deploy the service as a Compose file. Template import is planned.
- **Images built on CapRover** are not recoverable through its API, so the importer needs a repository or registry for each app.

## Move over safely

<Steps>
  <Step title="Install on a spare server">
    One script checks the host, installs Docker if missing and starts the control plane.
  </Step>
  <Step title="Dry-run the importer">
    Point it at your CapRover URL with --dry-run. The importer is read-only against the source.
  </Step>
  <Step title="Import and compare">
    Import a subset with --only, deploy it, and run both for a while before switching DNS.
  </Step>
</Steps>

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Does Levelrail need Docker Swarm?">
    No. It runs plain Docker containers on each node. Placement across servers is explicit, and cordon and drain are supported.
  </Accordion>
  <Accordion title="How do I migrate from CapRover?">
    Use levelrail-cli import platform caprover with your CapRover URL. It is read-only against the source and supports --dry-run and --only. Read [Migrate from CapRover](/migrate-from-caprover) for what maps and the downtime window.
  </Accordion>
  <Accordion title="Does it have a one-click app store?">
    Levelrail ships a catalog of one-click templates, each a Compose file the platform deploys. CapRover has its own one-click app store with a different set of apps. Check that the services you need are covered in either catalog.
  </Accordion>
  <Accordion title="Is it free?">
    Yes. Apache 2.0, one edition, nothing behind a paywall.
  </Accordion>
  <Accordion title="Does Levelrail support SSO, SAML and SCIM?">
    Levelrail has password, passkey and TOTP sign-in, OAuth sign-in with 20 providers and generic OpenID Connect, IAM policies, teams and a hash-chained audit log, all in the one free edition. SAML single sign-on and SCIM provisioning exist but are beta: they sit behind the `saml-scim` feature toggle, are off by default, and are tested against a test identity provider rather than a live Okta or Entra ID tenant. See [SAML and SCIM](/sso-saml). For what CapRover supports, see its own documentation.
  </Accordion>
</AccordionGroup>

## Try it beside your CapRover setup

Import read-only, run both, and keep what works.

[Get started](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)
