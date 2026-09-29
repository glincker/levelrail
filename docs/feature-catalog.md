---
description: Complete inventory of Levelrail's dashboard routes, API endpoints, CLI commands, and known implementation gaps.
---

# Feature catalog

What's actually built, as of this page's own last update, so new work
checks here before re-discovering or rebuilding something that already
exists. This is a snapshot, not a live-generated index: re-verify
against the real route/API surface before relying on it for anything
more than orientation, the same caveat `docs/roadmap.md` states for
itself.

Unlike `docs/roadmap.md` (a narrative status page), this is a flat
reference: every dashboard page, every API resource group, every CLI
command group, in one place. When in doubt about whether something has
a UI, an API, or a CLI surface, check the corresponding row here first.

## Dashboard pages

This section lists what you can see and do for each app, database, and system area. Descriptions focus on what the operator sees and controls, not the implementation.

### Apps (`/apps/$name/*`)

| Page | What you can do |
| --- | --- |
| Overview | See live metrics (CPU, memory, disk), app status, health check results, and automatic diagnosis when something fails |
| Deploys | View deploy history, inspect each deploy with its commit message and duration, watch live build logs as they stream in, or compare two deploys side by side |
| Deploy settings | Configure deploy strategy (rolling, blue-green, or recreate), add pre and post-deploy hooks (shell commands that run before and after deploy), and see the outcome from the last hook run |
| Domains | Add, remove, and manage custom domains pointing to this app, and view TLS certificate status for each |
| Environment | Add, edit, and delete environment variables; mark sensitive ones as secrets so their value is envelope-encrypted at rest and never written to app.yaml or the git repo |
| Exec | Run one-off commands inside running containers without stopping the app |
| Feature flags | Toggle app behavior at runtime without redeploying, and see which flag values are currently active |
| Health | Set up readiness and liveness probes so the platform knows when your app is ready to serve traffic and when it has crashed |
| Integrations | Send app logs to external log drains (Datadog, Papertrail, etc.) |
| Logs | Search and filter logs from running containers with full-text search, or tail live logs in real time |
| Metrics | View CPU, memory, disk I/O, network I/O, request rate, response times, error rate, and container restart counts over time |
| Network | See which node the app is running on and internal DNS names for communicating with other apps and databases |
| Resources | Set CPU and memory limits so the app does not starve others or consume unbounded resources; see platform recommendations based on actual usage |
| Scheduled tasks | Set up cron-like tasks that run inside the app on a schedule |
| Services | Manage multi-service apps: a web frontend plus a background worker, both under the same app.yaml |
| Source | Connect a git repository and configure branch-to-environment mapping, preview environments per pull request, and inspect webhook deliveries from your git provider |
| Volumes | Back up app storage volumes to S3, restore from a backup, or browse backup history |
| Alerts | Set up alerts that fire when metrics cross a threshold, and choose notification channels (email, Slack, Discord, Telegram) |

### Databases (`/databases/$name/*`)

| Page | What you can do |
| --- | --- |
| Overview | Manage backups and restore points, expose the database outside the Docker network for external tools, attach it to apps, and check TLS certificate status |
| Logs | View detailed activity logs from the database engine |
| Metrics | Monitor resource usage over time: CPU, memory, network I/O, and disk I/O |
| Resources | Set CPU and memory limits and see platform recommendations based on usage |

### System and organization

- **Nodes** - List all managed servers, check their health status, drain workloads before maintenance, prevent scheduling new apps on a node, and view node metrics
- **Cross-app domains** - Configure shared domain routing and TLS settings that apply to all apps using them
- **Projects** - Group apps and databases by project for better organization
- **Environments** - Create environment tiers (staging, production) and scope app settings and variables per environment
- **Organizations** - Set up multi-tenant structure for teams or separate business units

### Settings

| Category | What you can do |
| --- | --- |
| Account | Update your profile and preferences |
| Security | Enable two-factor authentication and TOTP |
| General | Check system health status, clean up unused Docker containers and volumes, view and rotate certificates, rotate the master encryption key |
| Tokens | Create and revoke API tokens for CLI and automation |
| CLI access | Set up CLI authentication |
| Users | Invite team members and manage accounts |
| IAM policies | Define granular access control rules for team members |
| Audit log | View all actions taken by any user, with filtering and exports |
| Backup targets | Configure S3 or S3-compatible storage for app and database backups |
| Registry credentials | Store Docker registry credentials for private image pulls |
| Container registry | Use the built-in container registry to host images |
| Notification channels | Set up email, Slack, Discord, or Telegram for alert notifications |
| Organizations | Manage organization-level settings |
| OAuth sign-in | Enable single sign-on via GitHub, Google, or other OAuth providers |
| Git provider apps | Connect GitHub Apps, GitLab integrations, and Bitbucket connections for automatic deployments |
| Cloudflare Tunnel | Route traffic through Cloudflare instead of opening ports directly |
| Vault | Connect external secret management (HashiCorp Vault) for credential storage |
| Email | Configure outgoing email for invites and notifications |
| System status | Run the doctor diagnostic, view the status page, and check the attention panel for critical issues |
| Containers | View all containers running on the control plane |
| Updates | Check and install platform updates |

## API resource groups (`internal/api/routes.go`, `routes_platform.go`)

396 registered routes total (see [api-reference.md](api-reference.md) for
the exact method/path/ability of every one), grouped by resource:

| Resource | Routes | Representative paths |
| --- | --- | --- |
| System (status/doctor/containers/prune/orphaned-volumes/master-key/firewall/onboarding/updates) | 14 | `GET /system/status`, `POST /system/prune`, `GET /system/volumes/orphaned`, `POST /system/master-key/rotate` |
| Auth/2FA/users/roles/IAM/device-auth/OAuth | 33 | `/auth/login`, `/auth/2fa/*`, `/iam/policies*`, `/auth/device/*` |
| Apps CRUD/lifecycle/deploy | 31 | `/apps`, `/apps/{name}/deploys`, `/restart`, `/exec`, `/deploy-spec`, `/hook-runs` |
| Apps at scale (filtered list, status summary, bulk actions, clone preview, promote diff) | 3 | `GET /apps?tag=&environment=&q=`, `GET /apps-summary`, `POST /apps/bulk`, `GET /apps/{name}/clone/preview` |
| Secrets / git-source / webhooks / previews | 12 | `/apps/{name}/secrets*`, `/webhooks/github/{name}`, `/previews*` |
| Telemetry (metrics/logs) | 10 | `/apps/{name}/metrics`, `/logs/stream`, `/logs/download` |
| Alerts / scheduled tasks / feature flags / notify channels | 22 | `/apps/{name}/alerts`, `/flags/evaluate/{key}`, `/notification-channels*` |
| Databases CRUD + engines + resources | 10 | `/database-engines`, `/databases/{name}/resource-recommendation` |
| Projects / orgs / environments (+ shared env layers) | 22 | `/projects*`, `/organizations/{id}/env` |
| Nodes | 11 | `/nodes`, `/{id}/cordon`, `/drain`, `/workloads` |
| Ingress / certs / domains / email / Cloudflare | 19 | `/certificates`, `/settings/ingress*`, `/settings/cloudflare-tunnel*`, `/domains/{domain}/tls-cert` |
| Static sites / backup targets / registry credentials | 15 | `/static-sites`, `/backup-targets*`, `/registry-credentials*` |
| Built-in container registry | 5 | `/settings/registry`, `/registry/repositories`, `/registry/tags` |
| Git provider apps (GitHub/GitLab/Bitbucket/Gitea) | 34 | `/github-app*`, `/gitlab-app*`, `/bitbucket-app*`, `/gitea-app*` |
| DB backups/restore/clone-restore | 17 | `/databases/{name}/backups*`, `/restore-as-new`, `/backup-schedule`, `/backups` |
| DB point-in-time restore (PITR, postgres only) | 7 | `/databases/{name}/pitr*`, `/base-backups*`, `/pitr-restore*` |
| App volume backups/restore | 11 | `/apps/{name}/volumes/{volume}/backups*` |
| Control plane self-backup (SQLite snapshots, scheduled and pre-upgrade; offline `restore-db`) | 4 | `/system/backups*` |
| Control plane disaster recovery (age-encrypted off-box backups, key escrow, `restore`, scheduled restore drills) | 4 | `/system/control-plane-dr*` |
| App storage/database attach | 5 | `/apps/{name}/storage`, `/apps/{name}/database` |
| Audit log / log-drain | 5 | `/audit-log`, `/audit-log/purge` |

## CLI command groups (`cmd/levelrail-cli/`)

All command groups available:

`apps`, `databases`, `auth`, `profile`, `tokens`, `domains`, `backups`, `pitr`, `app-volume-backups`, `cloudflare-tunnel`, `channels`, `backup-targets`, `registry-credentials`, `registry`, `flags`, `nodes`, `status`, `version`, `audit-log`, `audit-purge`, `doctor`, `attention`, `containers`, `system-prune`, `control-plane-backups`, `volumes-orphaned`, `volumes-orphaned-cleanup`, `firewall`, `users`, `iam`, `secrets`, `migrate`, `completion`, `settings`, `github-app`, `gitlab-app`, `bitbucket-app`, `gitea-app`, `templates`, `static-sites`, `tags`, `shared-env`.

### Key command groups

**apps**

`create`, `list`, `get`, `deploy`, `deploy-compose`, `deploy-spec`, `validate` (local app.yaml/compose parse, no API call), `group`, `hook-runs`, `rollback`, `auto-rollback`, `deploys`, `promote`, `restart`, `stop`, `start`, `delete`, `status`, `diagnose`, `resource-recommendation`, `network`, `logs` (with `--follow`/`-f` for live tail), `metrics`, `exec`, `log-drain`, `scheduled-tasks`, `alerts`, `organizations`, `projects`, `environments`, `previews`, `secrets`, `git-source`, `webhook-deliveries`, `storage`, `tag`, `untag`.

**databases**

`create`, `list`, `get`, `delete`, `resource-recommendation`, `metrics`.

**nodes**

`list`, `get`, `delete`, `join-token`, `cordon`, `uncordon`, `drain`, `workloads`, `health`, `patch-status`, `metrics`, `events` (connection history).

**iam**

`policies` with subcommands: `create`, `list`, `get`, `update`, `delete`, `attach`, `detach`.

**backups** / **app-volume-backups**

`list`, `list-all` (backups only; instance-wide across every database and app volume), `trigger`, `restore`, `restore-as-new`, `schedule`, `verify`, `verifications`.

**pitr** (postgres only)

`enable`, `disable`, `status`, `base-backups` (`list`, `trigger`), `restore`.

**migrate**

Support for migrating from `coolify`, `dokploy`, or `caprover`.

**domains**

`list`, `cloudflare-dns`, `route53-dns`, `basic-auth`, `maintenance`, `tls-cert`, `certificates` (with a RENEWAL column), `error-pages`.

**settings**

Headless first-run setup with no browser needed:
- `oauth` (list/set) - instance-wide OAuth sign-in
- `email` (get/set) - outbound email configuration
- `ingress` (get/set) - ingress and ACME configuration

**github-app** / **gitlab-app** / **bitbucket-app** / **gitea-app**

`repos` (or `projects` for gitlab-app), `branches`, `use-as-source`. Connecting the App/OAuth integration itself stays dashboard-only (browser redirect through the provider's OAuth flow). These subcommands let you browse and use an already-connected integration's repos from the CLI.

**templates**

`list`, `get`, `deploy` (deploys a catalog entry's compose.yaml as an app, the same call `apps deploy-compose` makes).

**static-sites**

`list`.

**tags**

`list`, `create`, `delete`, `apps` (list apps with a tag).

**shared-env**

`list`, `set`, `delete` (all scoped to `--scope project|organization|environment --id ID`).

## Known gaps (backend done, UI thin or missing)

Verified by grepping `web/src` for a matching component/query; a real
API/CLI capability with no dashboard surface counts as a gap here, not
a documentation issue, unless `docs/roadmap.md` explicitly claims
otherwise for that feature.

| Capability | API / CLI | Status |
| --- | --- | --- |
| Master key rotation trigger | `POST /system/master-key/rotate`, `secrets rotate-master-key` | Closed: `RotateMasterKeyDialog` on Settings > General |
| Container visibility | `GET /system/containers`, `containers` | Closed: Settings > Containers |
| Firewall sync trigger | `POST /system/firewall/sync`, `firewall sync` | In progress on a separate branch |

When closing a gap here, move its row into the relevant section above
instead of leaving it listed as both done and gapped.

## See also

- [API reference](api-reference.md) - Method/path/ability listing for the API routes (all 396 registered routes in `routes.go` and `routes_platform.go`, generated by `go run ./scripts/gen-api-reference`)
- [Roadmap](roadmap.md) - Current status and what's in progress
- [Getting started](getting-started.md) - Your first deploy walkthrough
