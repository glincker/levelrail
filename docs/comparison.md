---
title: vs Coolify, Dokploy, CapRover, Dokku, and Kamal
description: "Honest comparison of Levelrail with Coolify, Dokploy, CapRover, Dokku and Kamal: architecture, what each is good at, and what ships without a paywall."
"og:image": "https://docs.levelrail.com/public/og/comparison.png"
"twitter:image": "https://docs.levelrail.com/public/og/comparison.png"
"og:url": "https://docs.levelrail.com/comparison"
---

<div className="flex flex-wrap items-center gap-2 not-prose"><span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-white p-1.5 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/coolify/default.svg" alt="Coolify" title="Coolify" width="24" height="24" /></span><span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-white p-1.5 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/dokploy/default.svg" alt="Dokploy" title="Dokploy" width="24" height="24" /></span><span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-white p-1.5 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/caprover/default.svg" alt="CapRover" title="CapRover" width="24" height="24" /></span><span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-white p-1.5 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/dokku/default.svg" alt="Dokku" title="Dokku" width="24" height="24" /></span><span className="inline-flex h-9 w-9 items-center justify-center rounded-lg bg-white p-1.5 ring-1 ring-black/10"><img src="https://cdn.jsdelivr.net/gh/glincker/thesvg@a7c5ddd3f529accdf86519cc8fb96cb76cef4efd/public/icons/kamal/default.svg" alt="Kamal" title="Kamal" width="24" height="24" /></span></div>

This page compares architecture and feature breadth, not maturity. Many Levelrail feature areas are still labeled beta in [feature status](/feature-status).

This is positioning, not a ranking. Coolify, Dokploy, CapRover, Dokku and Kamal are all real projects with active users, and any of them may be the right choice for you. This page shows where Levelrail makes different design choices so you can decide.

## How to read this page

- Statements about other projects are limited to what their public documentation and repositories state. They can be out of date: these projects ship quickly. Check their own docs before deciding.
- Where something is uncertain, we say so.
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

Coolify has a large catalog of one-click services and a polished dashboard, and it manages servers you already have over plain SSH with nothing to install beyond Docker. If you want the broadest template list and are fine with SSH-driven management, it is a strong choice.

Dokploy builds on Docker Swarm, so [multi-node](/multi-node) scheduling and rolling updates come from a mature, widely understood mechanism, with a modern dashboard on top. If you already think in Swarm terms, it fits naturally.

CapRover is a long-running, simple, Swarm-based platform with a one-click app store and a CLI. It has a large install base and is easy to pick up.

Dokku is the classic "Heroku on your own box": `git push` to deploy, buildpacks, a rich plugin ecosystem, and almost no idle footprint because there is no long-running control service. For a single server it is hard to beat on simplicity. Levelrail's zero-DNS HTTPS is the closest counter.

Kamal is deliberately minimal: no control plane, no database, no agent. It runs over SSH from your CI and pairs with `kamal-proxy` for zero-downtime cutover. If you want as few moving parts as possible and don't need a dashboard, it is excellent.

## Where Levelrail differs

- An agent instead of SSH or Swarm. Managed nodes need no inbound ports: each agent dials out with mutual TLS, streams Docker events up, and talks to the Docker Engine API directly. Nothing shells out to the `docker` CLI.
- A reconciler, not a script. Desired state lives in the database, and a level-triggered loop converges the node toward it and writes a status condition with a reason after every pass. A deploy is reported healthy only after the new container's readiness probe passes.
- Observability in the core. Each node keeps its own metrics and logs (15 second resolution metrics, full-text log search), queried through the control plane. Alerting, crashloop detection with the last log lines, and a Prometheus remote-read endpoint are built in. See [Observability](/observability).
- A small footprint. SQLite in WAL mode, an embedded Caddy, and an embedded dashboard mean a single binary on a single node. See [Performance](/performance) for measured numbers.
- Rollback that survives cleanup. Prior images are pinned so garbage collection cannot remove a rollback target.
- AI-ready, not AI-driven. An MCP server exposes the same API and permission model to agents. AI is never in the reconcile path.

## SSO, audit, IAM and backups are not paywalled

Levelrail has one edition. It is Apache 2.0 licensed, there is no license key, no enterprise tier and no feature gated behind a plan. Everything below ships in the same binary for everyone:

- Sign-in: email and password, TOTP two-factor authentication, passkeys, and OAuth sign-in with 20 providers (Google, GitHub, Microsoft, GitLab and more) or any generic OpenID Connect provider. See [Identity and access](/identity-and-access).
- SAML single sign-on and SCIM provisioning: beta, and off by default behind the `saml-scim` feature toggle. See [SAML and SCIM](/sso-saml).
- IAM: AWS-style Allow and Deny policies scoped to `app:name`, `database:name` or `*`, attachable to users and API tokens, on top of three role presets (admin, operator, viewer). API tokens carry fine-grained abilities.
- Audit log: every mutating and sensitive request is recorded with actor, ability, method, path, status, remote address and caller surface (CLI, dashboard, MCP or API). It is queryable and exportable as CSV with configurable retention.
- Backups: scheduled database and volume backups with retention, automatic verification after each run, restore, and restore into a new resource. The control plane database gets automatic snapshots, a pre-migration snapshot, encrypted off-box backups and restore drills. See [Backups and storage](/backups-and-storage) and [Control plane backup](/control-plane-backup).
- Approvals and freezes: protected environments, deploy approvals and deploy freeze windows. See [Deploy safety](/deploy-safety).

| | Levelrail | Coolify | Dokploy | CapRover | Dokku | Kamal |
| --- | --- | --- | --- | --- | --- | --- |
| SSO (SAML, OIDC) | OIDC and 20 OAuth providers; SAML beta | Not documented here, see their docs | SSO provider configuration and enforcement exist in the codebase, see their docs for protocols | Not documented here, see their docs | Not documented here, see their docs | None: a command line tool with no web login |
| SCIM provisioning | Beta, off by default | Not documented here, see their docs | Not documented here, see their docs | Not documented here, see their docs | Not documented here, see their docs | None: no server component |
| Audit log and streaming | Hash-chained, CSV export, forwarding to syslog, a webhook or a bucket | Not documented here, see their docs | Not documented here, see their docs | Not documented here, see their docs | Not documented here, see their docs | None: no server component |

If SSO, audit logging or fine-grained roles matter to you, check each project's own pricing and licensing pages. We have not verified every project's current split, and it changes over time.

## What Levelrail does not do yet

- LDAP: OAuth and OIDC sign-in are supported, and SAML single sign-on and SCIM provisioning are beta (off by default, run against Keycloak and SimpleSAMLphp in a local lab, not tested against Okta or Entra ID); LDAP directory sync is not supported.
- The largest template catalog: Levelrail ships about 360 one-click templates, each a Compose file the platform deploys and manages. Coolify's catalog is in the same range and other projects may list more, and the end-to-end suite live-deploys a sample of Levelrail's templates, not all of them. See [Templates and registry](/templates-and-registry).
- Track record: fewer real-world deployments and less community knowledge than projects that have existed for years.
- Public ACME has had one live run: TLS defaults to an internal issuer. The public ACME issuer was verified once against Let's Encrypt on a public VPS. Renewal and DNS-01 wildcards are not yet exercised live. See the [roadmap](/roadmap).
- No Windows or non-Linux nodes, and no Kubernetes compatibility layer: these are deliberate non-goals.

## Choosing

- One server, want the simplest possible thing: Dokku or Kamal.
- Broadest template list and SSH-only server management: Coolify.
- Docker Swarm is already your model: Dokploy or CapRover.
- Want an agent-based control plane with built-in observability, IAM and audit on one binary: Levelrail.

To move an existing setup, see [Migrate to Levelrail](/migrate-to-levelrail).

## See also

- [Coolify alternative](/coolify-alternative), [Dokploy alternative](/dokploy-alternative), [Vercel alternative](/vercel-alternative), [Netlify alternative](/netlify-alternative), [Cloud Run alternative](/cloud-run-alternative), [Heroku alternative](/heroku-alternative) and [Railway alternative](/railway-alternative): focused guides for each switch
- [Architecture](/architecture): how Levelrail is built internally
- [Feature catalog](/feature-catalog): the dashboard pages by area
- [Cloud integrations](/cloud-integrations): what works today with AWS, Google Cloud, Cloudflare, DigitalOcean and Hetzner
- [Roadmap](/roadmap): what has shipped and what is open
