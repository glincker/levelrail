---
title: vs Coolify, Dokploy, CapRover, Dokku, and Kamal
description: "An honest comparison of Levelrail with Coolify, Dokploy, CapRover, Dokku, and Kamal: architecture, what each is good at, and what Levelrail ships without a paywall."
---

# Comparison

This page compares architecture and feature breadth, not maturity. Levelrail has no stable release yet, and most feature areas are labeled beta in [feature status](feature-status.md).

Positioning, not a ranking. Coolify, Dokploy, CapRover, Dokku and Kamal are all real, useful projects with active users, and any of them may be the right choice for you. This page explains where Levelrail makes different design choices, so you can decide.

## How to read this page

- Statements about other projects are limited to what their public documentation and repositories state. They can be out of date: these projects ship quickly. Check their own docs before deciding.
- Where something is uncertain we say so instead of guessing.
- Levelrail is the youngest project on this list. It has fewer users, fewer integrations and less production mileage than the others. See [What Levelrail does not do yet](#what-levelrail-does-not-do-yet).

## At a glance

| Project | How it manages servers | Orchestration | Ingress | Runs as |
| --- | --- | --- | --- | --- |
| Levelrail | A node agent that dials out to the control plane over gRPC with mutual TLS, talking to the Docker Engine API | A level-triggered reconciler that diffs desired and observed state | Caddy embedded in the control plane | One control plane binary plus one agent binary per extra node |
| Coolify | SSH from the control plane to each server | Docker and Docker Compose per resource | Traefik (Caddy is also offered, see their docs) | A web application with its own database, on your server |
| Dokploy | Docker Swarm for multi-node | Docker Swarm services and Docker Compose | Traefik | A web application with its own database, on your server |
| CapRover | Docker Swarm | Docker Swarm services | nginx | A web application running as a Swarm service |
| Dokku | Commands run on the host itself, driven over SSH or `git push` | A plugin-based scheduler over Docker on one host (other schedulers exist, see their docs) | nginx by default, other proxies via plugins | A set of scripts on the host, no separate web service |
| Kamal | SSH from your machine or CI to each server | None: it runs Docker commands per deploy | `kamal-proxy` | A command line tool, no server component |

## What each project is good at

**Coolify** has a large catalog of one-click services and a polished dashboard, and supports servers you already have over plain SSH with nothing to install on them beyond Docker. If you want the broadest template list and are comfortable with SSH-driven management, it is a strong choice.

**Dokploy** builds on Docker Swarm, so multi-node scheduling and rolling updates come from a mature, widely understood mechanism, with a modern dashboard on top. If you already think in Swarm terms it is a natural fit.

**CapRover** is a long-standing, simple, Swarm-based platform with a one-click app store and a CLI. It has a large install base and a gentle learning curve.

**Dokku** is the classic "Heroku on your own box": `git push` to deploy, buildpacks, a rich plugin ecosystem, and almost no idle footprint because there is no long-running control service. For a single server it is hard to beat for simplicity.

**Kamal** is deliberately minimal: no control plane, no database, no agent. It runs over SSH from your CI and pairs with `kamal-proxy` for zero-downtime cutover. If you want the smallest possible moving parts and do not need a dashboard, it is excellent.

## Where Levelrail differs

- **Agent instead of SSH or Swarm.** Managed nodes need no inbound ports: each agent dials out with mutual TLS, streams Docker events up, and talks to the Docker Engine API directly. Nothing shells out to the `docker` CLI.
- **A reconciler, not a script.** Desired state lives in the database; a level-triggered loop converges the node toward it and writes a status condition with a reason after every pass. A deploy is reported healthy only after the new container's readiness probe passes.
- **Observability in the core.** Each node keeps its own metrics and logs (15 second resolution metrics, full-text log search), queried through the control plane. Alerting, crashloop detection with the last log lines, and a Prometheus remote-read endpoint are built in. See [Observability](observability.md).
- **One small footprint.** SQLite in WAL mode, an embedded Caddy, and an embedded dashboard mean a single binary on a single node. See [Performance](performance.md) for measured numbers.
- **Rollback that survives cleanup.** Prior images are pinned so garbage collection cannot remove a rollback target.
- **AI-ready, not AI-driven.** An MCP server exposes the same API and permission model to agents. AI is never in the reconcile path.

## SSO, audit, IAM and backups are not paywalled

Levelrail has one edition. It is Apache 2.0 licensed, there is no license key, no enterprise tier and no feature gated behind a plan. Everything below ships in the same binary for everyone:

- **Sign-in.** Email and password, TOTP two-factor authentication, and OAuth sign-in with Google, GitHub or any generic OpenID Connect provider. See [Identity and access](identity-and-access.md).
- **IAM.** AWS-style Allow and Deny policies scoped to `app:name`, `database:name` or `*`, attachable to users and API tokens, on top of three role presets (admin, operator, viewer). API tokens carry fine-grained abilities.
- **Audit log.** Every mutating and sensitive request is recorded with actor, ability, method, path, status, remote address and caller surface (CLI, dashboard, MCP or API). It is queryable and exportable as CSV with configurable retention.
- **Backups.** Scheduled database and volume backups with retention, automatic verification after each run, restore, and restore into a new resource. The control plane database gets automatic snapshots, a pre-migration snapshot, encrypted off-box backups and restore drills. See [Backups and storage](backups-and-storage.md) and [Control plane backup](control-plane-backup.md).
- **Approvals and freezes.** Protected environments, deploy approvals and deploy freeze windows. See [Deploy safety](deploy-safety.md).

Some other platforms in this list reserve features such as SSO, audit logging or fine-grained roles for a paid plan or hosted offering. We have not verified every project's current split, and it changes over time, so check each project's pricing and licensing pages if this matters to you.

## What Levelrail does not do yet

- **SAML and SCIM.** OAuth and OIDC are supported; SAML single sign-on and SCIM provisioning are not.
- **A large template catalog.** Levelrail ships a curated set of one-click templates rather than hundreds. Other projects, Coolify in particular, offer far more.
- **Track record.** Fewer real-world deployments and less community knowledge than projects that have existed for years.
- **Public ACME issuance is not yet proven against every environment.** TLS defaults to an internal issuer and a public ACME issuer is built, with less field verification than the rest of the ingress. See the [roadmap](roadmap.md) for current status.
- **No Windows or non-Linux nodes, and no Kubernetes compatibility layer.** These are deliberate non-goals.

## Choosing

- One server, want the simplest possible thing: Dokku or Kamal.
- Broadest template list and SSH-only server management: Coolify.
- Docker Swarm is already your model: Dokploy or CapRover.
- Want an agent-based control plane with built-in observability, IAM and audit on one binary: Levelrail.

To move an existing setup, see [Migrating from Coolify, Dokploy and CapRover](migrating-from-coolify-dokploy-and-caprover.md).

## See also

- [Architecture](architecture.md) - How Levelrail is built internally
- [Feature catalog](feature-catalog.md) - Complete inventory of routes, API, CLI
- [Roadmap](roadmap.md) - Development status and what is coming next
