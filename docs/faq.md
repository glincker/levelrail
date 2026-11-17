---
title: "Frequently asked questions"
sidebarTitle: "FAQ"
description: Short answers to the most common questions about Levelrail, a self-hosted deployment platform, with a docs link and a stable, beta or experimental label where one applies.
"og:url": "https://docs.levelrail.com/faq"
---

Levelrail, a self-hosted deployment platform, runs on your own Linux servers. The answers below are as of 1.4.0. Labels follow [Feature status](/feature-status): a feature with no label there is core and on by default.

## What is Levelrail?

Levelrail is a lightweight, self-hosted deployment platform. The control plane is one Go binary that idles at about 200 MB of RAM and 1.5 percent of one CPU on a 2 vCPU box; your apps use their own resources on top. For the full list by area, see [All features](/features).

## Does Levelrail need Kubernetes?

No. Levelrail runs apps as Docker containers on Linux servers and has no Kubernetes requirement. Connecting an existing Kubernetes cluster and deploying to it is experimental and off by default. See [Import Kubernetes manifests](/import-kubernetes) and [Feature status](/feature-status).

## How do I install it?

Run the install script as root on a Linux server (amd64 or arm64, with systemd). It installs Docker if missing, verifies the release checksum, starts the `levelrail` service and prints a sign-in link. 1 vCPU and 1 GB of RAM boots it. See [Getting started](/getting-started).

## How much memory does it use?

About 200 MB of RAM and 1.5% of one CPU when idle, measured on a 2 vCPU box on 2026-10-19. The benchmark method is published so you can repeat it on your own hardware. See [Benchmarks](/benchmarks).

## How do I deploy with git push?

Connect a repository from GitHub, GitLab, Bitbucket, Gitea or Forgejo and each push deploys. To push straight to your own server as a git remote, turn on Git push deploy in Settings, Beta features. That remote is beta and off by default. See [Git push deploy](/git-push-deploy) and [Git integrations](/git-integrations).

## How do preview environments per pull request work?

Levelrail deploys a copy of the app with its own URL when a pull request opens, redeploys it on new commits and removes it when the pull request closes or merges. Each app opts in. Stable on GitHub, beta on GitLab and Bitbucket. See [Preview environments](/preview-environments).

## How many templates are in the catalog?

570+ templates in the catalog. Exact split as of 1.4.0: 574 catalog entries (565 active, 9 deprecated), in 22 categories. 346 have a recorded pass on a real engine, and 228 are marked preview, which means installable with no recorded pass yet. Templates are labeled beta. See [Template catalog](/template-catalog).

## How do I add a server as a node?

Create a single-use join token in the dashboard or CLI and run the agent on any Linux host. The agent dials out to the control plane and enrolls over mutual TLS, so the node needs no inbound ports. Multi-node is beta. See [Add a node anywhere](/add-node-anywhere).

## Can Levelrail create servers at a cloud provider?

Yes, for six providers: Hetzner, DigitalOcean, AWS, Azure, GCP and Oracle Cloud. It creates the server and enrolls it as a node. This is beta, and AWS and Azure are not yet verified on real accounts. See [Node provisioning](/node-provisioning) and [Cloud integrations](/cloud-integrations).

## Which databases can it run?

13 engines are in code: Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB, Dragonfly, ClickHouse, Valkey, Meilisearch, Qdrant, Typesense and Memcached. 9 are live-verified. The last four in that list are built but not yet live-verified. See [Managing databases](/managing-databases).

## How do I back up and restore a Postgres database?

Add a backup target (S3-compatible, R2 or local), schedule backups, then restore in place or restore into a new database with `levelrail-cli backups restore` or `restore-as-new`. Postgres backups and point-in-time restore are stable. See [Managing databases](/managing-databases) and [Backups and storage](/backups-and-storage).

## How do I know a backup actually works?

Run a restore drill. It restores a real backup into a scratch resource and records the result, and scheduled drills run weekly by default. Restores also check the stored object against its recorded checksum first. See [Prove your backups work](/prove-your-backups-work).

## How does automatic HTTPS work?

An embedded Caddy ingress requests certificates through HTTP-01, and DNS-01 for wildcards. Ports 80 and 443 must be reachable. Public ACME is beta: it has one recorded live run, and renewal and wildcards are not yet verified live. See [Domains and ingress](/domains-and-ingress) and [Zero to HTTPS](/zero-to-https).

## Can I roll back a bad deploy?

Yes. Rollbacks go to a pinned prior image, and rollouts only cut over after the new container passes its readiness check. Post-deploy watch can roll back by itself on a 5xx spike or crashloop (stable). See [Zero-downtime deploys](/zero-downtime-deploys) and [Deploy safety](/deploy-safety).

## Do I need a separate monitoring stack?

No. Every app gets CPU, memory, network, disk and request metrics, full-text log search and alert rules without setup. Alerts go to 18 channel kinds. Webhook and email are stable, the 15 vendor kinds are beta. See [Observability](/observability).

## Which sign-in and access controls are included?

Passwords, passkeys, TOTP, OAuth with 20 providers plus generic OIDC (beta), IAM allow and deny policies, teams and a hash-chained audit log. SAML and SCIM are beta and off by default. See [Identity and access](/identity-and-access).

## Does it work with coding agents?

Yes. The MCP server exposes 259 tools, with a small core list by default and more through toolsets. Writes need explicit approval. The `levelrail-cli` command line and HTTP API cover the same operations. See [MCP](/mcp) and [CLI reference](/cli-reference).

## What does stable, beta and experimental mean?

Stable means tests plus a live end-to-end run on one machine. Beta works and is tested but has no end-to-end run or real infrastructure record. Experimental is off by default. Single node is stable. See [Feature status](/feature-status).

## Can I move from Coolify, Dokploy, CapRover or Dokku?

Yes. Guides cover each of them, using read-only API access where the source offers one. For an honest side-by-side of architecture and trade-offs, including what Levelrail does not do yet, see [Migrate to Levelrail](/migrate-to-levelrail) and [Comparison](/comparison).

## What license does Levelrail use?

Apache 2.0, with one edition and no license key. Every feature ships in the same binary. See [Comparison](/comparison).
