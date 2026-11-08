---
title: "Feature coverage"
description: What Levelrail covers, one row per capability, with a status label and a link to the page that explains it. Written from the code and the existing docs.
"og:url": "https://docs.levelrail.com/feature-coverage"
---

This page lists what Levelrail does, one row per capability, and where to read more. It is written from the source code and the existing docs. It does not compare Levelrail with other products.

Most rows say **Built and tested**. That means the code is in the repository and unit or integration tests cover it. It does not mean the feature ran on a real server. Only a row marked **Proven on a real host** cites a recorded run on a real machine. For the evidence behind each area (which tests exist, which live runs happened, what is still unproven), read [Feature status](/feature-status).

## Status labels

| Status | Meaning |
| --- | --- |
| **Built and tested** | The code exists and has unit or integration tests. No real host run is cited. This is the default for a row that has no stated run. |
| **Proven on a real host** | A run on real infrastructure is recorded in the docs. The row says what was and was not covered. |
| **Beta** | Works and is tested, but has no end-to-end test and no record of a run on real infrastructure. This is the beta label in [Feature status](/feature-status). Some beta features also sit behind a Settings switch. The linked page says which. |
| **Experimental** | Listed as experimental in the experimental catalog (`internal/experimental/catalog.go`). Off by default and gated behind a switch. See [Experimental features](/experimental-features). |
| **Plan only** | Planning or plan generation exists, but the build or deploy path has not run live. |

The labels on this page are a summary. [Feature status](/feature-status) holds the evidence tiers behind them.

## Deploy sources

| Capability | Status | Read more |
| --- | --- | --- |
| Git push to a remote on your own server, with a per-app token | Beta | [Git push deploy](/git-push-deploy) |
| Git push remote that keeps its token out of `.git/config` (`--credential-helper`) | Beta | [Git push deploy](/git-push-deploy) |
| Git provider webhooks for GitHub, GitLab, Bitbucket and Gitea | Built and tested | [Git integrations](/git-integrations) |
| Hold an automatic git deploy until the commit's CI checks finish | Built and tested | [CI gate](/ci-gate) |
| CI deploy trust through OIDC for GitHub Actions, GitLab CI and CircleCI | Built and tested | [Pipelines and OIDC](/pipelines-oidc) |
| SSH deploy key for private git repositories | Built and tested | [Git integrations](/git-integrations) |
| Container image deploy, with promotion by digest and signature checks | Built and tested | [Images](/images) |
| Docker Compose file as one app, with per-service status | Beta | [Compose apps](/compose-apps) |
| Upload a folder or a zip to a static site | Built and tested | [Drop deploy](/drop-deploy) |
| Deploy the current directory from the CLI, with no git remote | Built and tested | [Deploy from the CLI](/deploy-from-cli) |
| Import from a repo URL, a `docker run` command, an image, a compose file or a Dockerfile | Built and tested | [Importing apps](/importing-apps) |
| Levelrail MCP server for agent clients (local, tool based) | Built and tested | [MCP](/mcp) |
| Remote MCP endpoint at `POST /mcp`, read only, bearer token, no OAuth yet | Experimental | [MCP](/mcp) |

## Builds

| Capability | Status | Read more |
| --- | --- | --- |
| Dockerfile build | Built and tested | [Deploying apps](/deploying-apps) |
| Static site with no container, served with TLS by the embedded ingress | Built and tested | [Static sites](/pages) |
| Procfile and `app.json` translation into an app plan | Built and tested | [Doctor and import](/doctor-and-import) |
| Railpack build for Node, Go, Java and Python | Built and tested | [Frameworks](/frameworks) |
| Railpack build for Ruby | Plan only | [Importing apps](/importing-apps) |
| Railpack build for PHP | Plan only | [Importing apps](/importing-apps) |
| Framework presets with port, health path and storage hints (Next.js, Go, React and Vite, Spring Boot and more) | Built and tested | [Frameworks](/frameworks) |

Ruby and PHP projects get a plan from Railpack, but the docs say no live build has run for either. Other providers Railpack detects are rejected with an error.

## Databases and backups

| Capability | Status | Read more |
| --- | --- | --- |
| Managed databases for postgres, redis, mysql, mongodb, mariadb, keydb, dragonfly, clickhouse, valkey, meilisearch, qdrant, typesense and memcached | Built and tested | [Managed databases](/managed-databases) |
| Restore a dump into a database from a file or stdin, through the CLI and API. A database with data is refused unless forced | Beta | [Managing databases](/managing-databases) |
| Version upgrades with security checks and automatic rollback on failure | Built and tested | [Database upgrades](/database-upgrades) |
| TLS for Postgres and Redis databases, and Valkey once switched on | Built and tested | [Managing databases](/managing-databases) |
| TLS on new MySQL, MariaDB and MongoDB instances, and on older ones through the API | Beta | [Managing databases](/managing-databases) |
| Point-in-time restore for Postgres | Built and tested | [Backups and storage](/backups-and-storage) |
| Point-in-time restore for MySQL and MariaDB, from binlog | Beta | [Backups and storage](/backups-and-storage) |
| MongoDB dump, verify and restore as a new database (no point-in-time restore) | Beta | [Backups and storage](/backups-and-storage) |
| Backup targets referenced by ID, with schedules and retention | Built and tested | [Backups and storage](/backups-and-storage) |
| Scheduled dumps of compose database services (Postgres, MySQL and MariaDB images) | Beta | [Compose apps](/compose-apps) |
| Encrypted database dumps, sealed per target with age | Beta | [Backups and storage](/backups-and-storage) |
| Restore drills that prove a backup can be restored | Built and tested | [Prove your backups work](/prove-your-backups-work) |

Compose database dumps run on a cron schedule. Restoring one is a manual step today. The compose apps page lists the commands.

## Networking and ingress

| Capability | Status | Read more |
| --- | --- | --- |
| Custom domains with automatic TLS over HTTP-01 | Proven on a real host | [ACME verification runbook](/acme-verification-runbook) |
| Wildcard certificates through DNS-01 | Built and tested | [Domains and ingress](/domains-and-ingress) |
| Cloudflare Tunnel, with no open inbound port | Experimental | [Cloudflare Tunnel](/cloudflare-tunnel) |
| Load balancing across an app's replicas, with health checks and sticky sessions | Experimental | [Load balancing](/load-balancing) |
| Forward auth in front of a domain | Built and tested | [Domains and ingress](/domains-and-ingress) |
| Per-domain request body cap and upstream read and write timeouts | Built and tested | [Domain traffic controls](/domain-traffic-controls) |
| WAF, rate limiting and redirects | Built and tested | [Domains and ingress](/domains-and-ingress) |
| Abuse limits as runtime settings (`abuse.*`). The ingress limiter still reads environment values, so saved values do not change it yet | Beta | [Ingress abuse defense](/abuse-defense) |
| Private networks that group apps and allow named app-to-app paths | Built and tested | [Private networks](/private-networks) |
| WireGuard mesh between nodes | Built and tested | [Multi-node](/multi-node) |
| Mail DNS records (SPF, DKIM, DMARC, MX), add-only by default | Experimental | [Mail DNS](/mail-dns) |

The Proven on a real host row covers one recorded issuance, on 2026-10-05, against a public CA on a public VPS, using the staging CA first and then production. Renewal, DNS-01 wildcard issuance and the port 80 preflight from outside the server are not verified. See [the ACME runbook](/acme-verification-runbook) for the record.

## Previews

| Capability | Status | Read more |
| --- | --- | --- |
| Container pull request previews with their own URL, caps and optional database | Built and tested | [Preview environments](/preview-environments) |
| Static site pull request previews, named `<app>-pr-<number>` | Built and tested | [Preview environments](/preview-environments#static-sites) |
| Preview TTL, cap, idle sleep and cleanup when the pull request closes | Built and tested | [Previews](/previews) |
| Basic auth gate on previews (`--gate`) | Built and tested | [Preview environments](/preview-environments) |
| Previews list badge for the basic auth gate. It shows the app-wide policy on rows with a domain, not each domain's live state | Beta | [Previews](/previews) |
| Static previews: the basic auth gate is applied, and a publish without it is refused and marked failed | Built and tested | [Preview environments](/preview-environments#static-sites) |
| Static previews: a forward-auth gate on the preview domain is refused | Built and tested | [Preview environments](/preview-environments#static-sites) |

## Security

| Capability | Status | Read more |
| --- | --- | --- |
| HashiCorp Vault and OpenBao secret references (KV v2, token or AppRole) | Built and tested | [Vault secret references](/vault-secret-references) |
| Vault Transit engine wraps the master key | Built and tested | [Vault secret references](/vault-secret-references) |
| Doppler secret references (read only) | Beta | [Vault secret references](/vault-secret-references) |
| Infisical secret references (read only) | Beta | [Vault secret references](/vault-secret-references) |
| Secret masking and reveal policies for app variables | Built and tested | [Environment and secrets](/environment-and-secrets) |
| Audit log of every mutating request, with a hash chain you can verify and CSV export | Built and tested | [Audit log](/audit-log) |
| Team roles: owner, admin, developer and viewer | Built and tested | [Teams and roles](/teams-and-roles) |
| Custom roles, the guest role and per-environment grants | Experimental | [Access control](/access-control) |
| IAM policies on apps, databases and environments | Built and tested | [Access control](/access-control) |
| Global environments and the environment switcher | Experimental | [Environments](/environments) |
| Sign-in with OAuth providers and generic OpenID Connect, with group to role mapping | Built and tested | [SSO with OIDC](/sso-oidc) |
| SAML 2.0 single sign-on and SCIM 2.0 provisioning | Beta | [SSO with SAML](/sso-saml) |
| Container runtime options per service: read-only root, capability drop, pids, tmpfs, ulimits, init and stop timeout | Built and tested | [Runtime options](/runtime-options) |

## Multi-server

| Capability | Status | Read more |
| --- | --- | --- |
| Add a Linux machine over SSH, with Docker installed if needed and the node agent | Built and tested | [Add a node over SSH](/add-node-over-ssh) |
| Agent nodes with reverse-dialed gRPC and mTLS | Built and tested | [Multi-node](/multi-node) |
| Taints and tolerations, and affinity between apps | Built and tested | [Node scheduling](/node-scheduling) |
| Spread or bin-pack placement scoring | Built and tested | [Node scheduling](/node-scheduling) |
| Resource-aware placement with an overcommit policy | Built and tested | [Resource-aware placement](/resource-aware-placement) |
| Autoscale an app between a minimum and a maximum replica count | Built and tested | [Autoscaling](/autoscaling) |
| Migrate apps with their volumes between nodes | Built and tested | [Multi-node](/multi-node) |
| Data plane keeps serving routes while the control plane is stalled | Built and tested | [Resilience](/resilience) |

## App controls

| Capability | Status | Read more |
| --- | --- | --- |
| Run a one-off command in a fresh container from the app's Exec tab | Beta | [Jobs](/jobs) |
| Per-service replica count and restart on the Services tab of multi-service apps | Beta | [Deploying apps](/deploying-apps) |
| Service selector on the Logs tab of multi-service apps | Beta | [Log explorer](/log-explorer) |

## Observability

| Capability | Status | Read more |
| --- | --- | --- |
| Log search, saved queries and error signatures | Built and tested | [Log explorer](/log-explorer) |
| Log archive | Built and tested | [Log archive](/log-archive) |
| Node-local metrics, with federated queries and no central ingestion | Built and tested | [Observability](/observability) |
| Prometheus scrape | Built and tested | [Prometheus scrape](/prometheus-scrape) |
| OpenTelemetry traces, kept on the node and shown as a waterfall | Built and tested | [Tracing](/tracing) |
| Alerts and an in-app alert inbox | Built and tested | [Alert inbox](/alert-inbox) |
| Notification channels: webhook and email over SMTP | Built and tested | [Email notifications](/email-notifications) |
| Public status page | Built and tested | [Status page](/status-page) |

## Other gated features

These are the rest of the experimental catalog. Each one is switched on separately. The other seven catalog entries appear in the sections above.

| Capability | Status | Read more |
| --- | --- | --- |
| Plan, apply and export platform resources from files | Experimental | [Platform as code](/platform-as-code) |
| Plugins: signed, operator-approved, run in locked-down containers | Experimental | [Plugins](/plugins) |
| Host AI models on GPU nodes | Experimental | [AI models](/ai-models) |
| Admin mode for governing what agents and AI tools may do | Experimental | [AI control](/ai-control) |
| In-app AI assistant chat with your own model key | Beta | [AI assistant chat](/ai-assistant-chat) |
| Resource advisor: limit, tuning and idle suggestions, no automatic changes | Beta | [Resource advisor](/resource-advisor) |
| CLI console in the dashboard (never a host shell) | Beta | [CLI console](/cli-console) |
| Mobile API: phone pairing, device tokens and an overview endpoint | Beta | [Mobile API](/mobile-api) |
| Canvas: compose apps, databases and connections, review a plan, then apply | Beta | [Canvas](/canvas) |

## Keeping this page current

When a capability changes, update its row in the same change. The Experimental labels should match `internal/experimental/catalog.go`, and Beta labels follow [Feature status](/feature-status). A new Proven on a real host row needs a recorded run in the docs first.
