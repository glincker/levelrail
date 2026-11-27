---
title: "What is Levelrail"
description: "Levelrail is a self-hosted control plane that runs apps, databases, domains, TLS, backups and logs on your own servers or cloud accounts, with CLI, API and MCP."
"og:url": "https://docs.levelrail.com/what-is-levelrail"
---

Levelrail is an open source (Apache 2.0), self-hosted deployment platform. It runs as one Go binary and turns one or more Linux servers, including servers in your own cloud accounts, into a private cloud. You push code or a Docker image, and it builds, deploys, secures and watches the app.

## The problem

Running your own apps is more than starting a container. A production setup needs:

- a way to build and deploy on every push, and to roll back
- databases with scheduled, verified backups
- domains, DNS records and TLS certificates
- logs, metrics and alerts
- secrets, access control and an audit trail
- a way to repeat all of it on a second server

On your own servers or cloud accounts this usually means gluing together ten tools: a reverse proxy, a certificate client, a CI runner, a backup script, a log shipper, a metrics stack, a secrets store and a dashboard. Each has its own config, upgrades and failure modes.

## What Levelrail introduces

<CardGroup cols={2}>
  <Card title="One control plane" icon="/assets/icons/tree-structure.svg" href="/architecture">
    One binary with an embedded ingress and a small node agent. A single server needs nothing else.
  </Card>
  <Card title="Git deploy and rollback" icon="/assets/icons/cloud-arrow-up.svg" href="/git-push-deploy">
    Dockerfile, Compose and Railpack builds. A new container takes traffic only after its readiness check passes. See also [Rollback](/rollback).
  </Card>
  <Card title="Templates" icon="/assets/icons/rocket-launch.svg" href="/template-catalog">
    570+ catalog entries you install with one command or one click.
  </Card>
  <Card title="Databases" icon="/assets/icons/shield-check.svg" href="/managed-databases">
    13 engines with scheduled backups and restore checks.
  </Card>
  <Card title="Preview environments" icon="/assets/icons/cloud-arrow-up.svg" href="/preview-environments">
    A copy of an app per pull request, with its own URL and automatic teardown.
  </Card>
  <Card title="Observability" icon="/assets/icons/terminal-window.svg" href="/observability">
    Metrics, searchable logs, monitors and alerts stored on your own nodes.
  </Card>
</CardGroup>

### AI control plane

The same API is reachable from the dashboard, the [CLI](/cli-reference), the [API](/api-reference), [Terraform](/terraform-provider) and an MCP server for AI agents ([MCP tools](/mcp)). Agents use tokens with the same permissions and IAM policies as people. The [AI control](/ai-control) setting limits what agents may do, from off to admin, and the server enforces it on every request. Sensitive requests are recorded in the [audit log](/audit-log).

### Runtime settings applied live

Tunables are settings you change from the dashboard, CLI or API, and they apply without a restart. Environment variables only seed the defaults. The list is in [Runtime settings](/runtime-settings).

## Who it is for

- Solo developers and small teams who run apps on a VPS or two and want HTTPS, backups and rollbacks without writing scripts.
- Agencies that host many client apps and need teams, roles, previews and per-project cost views.
- Teams that already run their own servers or cloud accounts and want one place to deploy, observe and audit them, with Terraform and the API for automation.
- People who run AI tools and agents and want them to act through a permissioned, audited API.

See [Who Levelrail is for](/use-cases) for detail, and [Feature status](/feature-status) for the stable, beta or experimental label of every feature.

## Where to go next

- [Getting started](/getting-started): install and deploy your first app.
- [Compared with other approaches](/compared-with-other-approaches): what you do yourself and what Levelrail does.
- [Lower infrastructure cost](/lower-infrastructure-cost): where cost drops.
- [Leave anytime](/leave-anytime): export an app as plain Docker Compose.
