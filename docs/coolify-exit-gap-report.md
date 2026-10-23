---
title: "Coolify exit gap report"
sidebarTitle: "Coolify exit gaps"
description: What the Coolify importer carries over, what it reports for you to finish, and what it cannot see yet, audited against what a real Coolify instance holds.
"og:url": "https://docs.levelrail.com/coolify-exit-gap-report"
---

This is the audit behind the [Coolify exit plan](/migrate-from-coolify#exit-report-and-cutover-checklist). It compares what a Coolify v4 instance holds with what the importer reads, and says for each item whether it moves, is reported, or is invisible to the tool.

## How far to trust this

No real Coolify instance was available while this was written. The importer was checked against JSON fixtures shaped from Coolify's documented API responses, with fake credentials. That proves the mapping logic, the read-only guarantee (the fixture server rejects every non-GET) and the report. It does not prove that a field name or route matches your Coolify version. Field names such as `is_build_time`, `manual_webhook_secret_github` and the `/databases/{uuid}/backups` route are best knowledge, and a read that fails softly shows up as a note on the report instead of stopping it.

Before you delete Coolify, run `migrate coolify --dry-run` against your own instance, then compare the output with the Coolify dashboard for two or three apps you know well.

## Audit

Status words: **moves** means created on the new side, **reported** means the tool names it and you finish it, **invisible** means the tool does not read it at all.

| Coolify holds | Status | Notes |
| --- | --- | --- |
| Applications from git (Dockerfile, Nixpacks, static) | moves | Created stopped. Nixpacks becomes Railpack and is flagged because the build may differ. Connecting a private repo is a manual step |
| Applications from a registry image | moves | Image and tag kept |
| Docker Compose applications and one-click services | reported | Listed as skipped with the reason. Deploy the compose file with [compose apps](/compose-apps) |
| Environment variables and secrets | moves | Secret-looking names go through the encrypted store. Preview duplicates are dropped. Secrets the token could not read are listed by name |
| Build-time only flag on variables | reported | Variables are imported as runtime variables, and the names are listed |
| Install, build and start commands | reported | Silently dropped before this audit. Now listed with their values |
| Pre and post deployment commands | reported | Listed |
| Publish directory of a static site | reported | Listed |
| Domains | moves | Read, then held back until cutover. A `www` redirect rule is reported |
| TLS certificates | moves | Issued on first use after DNS points here. Custom uploaded certificates are not exported by Coolify's API |
| HTTP basic auth on an app | reported | New. A staged app would otherwise be public |
| Custom container labels (often Traefik middleware) | reported | Redirects, headers and rate limits need recreating as ingress settings |
| Custom docker run options | reported | Capabilities, devices, ulimits |
| Persistent volumes | moves | Created empty. The data copy is a separate step with file and byte counts |
| File mounts | reported | Content is not copied |
| Databases: PostgreSQL, MySQL, MariaDB, MongoDB, Redis | moves | Created empty at the same major version. Data copy uses `migrate db-copy`, and the report prints the command with the public port, user and database name read from Coolify |
| Databases: KeyDB, Dragonfly, ClickHouse | reported | Silently dropped before this audit. Now listed as skipped with the dump tool to use |
| Database backup schedules | reported | New. One row per schedule, with a note when it also uploads to S3 |
| Scheduled tasks (cron) | moves | Was report-only. Enabled tasks become scheduled jobs that run `sh -c COMMAND` inside the app. Disabled tasks are skipped. Staged apps get their jobs disabled until cutover so a job never runs on both hosts |
| Git webhook secrets (auto deploy) | reported | New. The git host still calls Coolify until you add a webhook pointing here |
| HTTP health checks | moves | Path, interval, timeout and retries |
| Command health checks | reported | Only HTTP probes exist here |
| Memory and CPU limits | moves | Swap and reservations are not mapped |
| Preview deployments | reported | Flag only. Previews are not recreated |
| Projects and environments | partial | The project becomes a project. The environment name is recorded in the report but several environments of one project share it |
| Servers and destinations | reported | New. The report lists every server, the proxy that holds ports 80 and 443, and warns when there is more than one deploy server. Staged apps land on the control plane node |
| Docker networks | invisible | Apps that rely on a custom Coolify network are not detected |
| Team members, roles, API tokens | invisible | Not migrated by design. Recreate users and tokens here |
| S3 storage destinations, SSH keys, GitHub App credentials | invisible | Credentials are never read. Add them again |
| Notification channels (Discord, Slack, email) | invisible | Recreate them as [alert channels](/alert-inbox) |
| Deploy history, logs, metrics | invisible | Not migrated |

## What the exit plan adds

- `migrate coolify --dry-run` prints a precise plan, one row per resource with its outcome: **imported**, **manual** or **skipped**, each with the reason and the next step.
- The same report is available over the API, in the dashboard import flow and as a read-only MCP tool.
- A DNS switch order (stateless apps first, apps with a volume and a database last) and an ordered cutover checklist that keeps Coolify as the fallback until you have verified.
- `ready_to_retire` is true only when every resource is imported and nothing needs a person. Skipped items always block it.

## Not proven yet

- A run against a real Coolify instance, including large instances with many environments.
- That Coolify's API returns the fields above for every build pack and version.
- Restoring a backup copied across, and the write pause needed for a database that takes writes during the final copy.
- Fallback to Coolify after the first write lands on the new database. Copy the data back first.

## Next gaps, by value

1. Map Coolify environments to Levelrail environments instead of one project.
2. Import database backup schedules and S3 targets instead of listing them.
3. Detect apps that depend on a custom Docker network.
4. Per-server node placement for multi-server instances.
5. Recreate git webhooks through the git provider integration.
