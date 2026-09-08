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

## Dashboard pages (`web/src/routes/`)

Every route is a thin wrapper (loader + a real component from
`web/src/components/`); a short route file is not itself a sign of a
stub, the substance lives in the imported component.

### Apps (`/apps/$name/*`)

| Page | Component |
| --- | --- |
| Overview | Hero, conditions, `DiagnosisPanel` |
| Deploys | History, live/replay log stream, `deploys/compare` |
| Deploy settings | Strategy config, `HooksEditor` (pre/post-deploy commands + last run outcome) |
| Domains | `DomainEditor` |
| Environment | Env var editor |
| Exec | `ExecPanel` (one-off container exec) |
| Feature flags | `FeatureFlagsPanel` |
| Health | Health-check editor |
| Integrations | `LogDrainCard` |
| Logs | `LogSearchPanel` |
| Metrics | `MetricsDashboard` |
| Network | `AppNetworkPanel` |
| Resources | `ResourceLimitsEditor` + recommendation |
| Scheduled tasks | `ScheduledTasksPanel` |
| Services | Multi-service group view |
| Source | `GitRepoSourcePicker`, `PreviewEnvironmentsCard`, `WebhookDeliveriesPanel` |
| Volumes | `AppVolumeBackupsSection` |
| Alerts | `AlertRulesPanel`, `DeployNotifyTargetsPanel` |

### Databases (`/databases/$name/*`)

Overview (backups, public access, attachment), logs, metrics, resources.

### System / org structure

Nodes (list + detail: health, cordon/drain, workloads toggle, metrics),
cross-app domains, projects, environments, organizations.

### Settings (`/settings/*`)

Account, security (2FA/TOTP), general (system status, Docker cleanup,
certificates, master key rotation), tokens, CLI access, users,
IAM policies, audit log (+ purge), backup targets, registry credentials,
notification channels, organizations, OAuth sign-in, GitHub/GitLab/
Bitbucket apps, Cloudflare Tunnel, email, system status (doctor bundle),
containers, updates.

## API resource groups (`internal/api/routes.go`, `routes_platform.go`)

266 registered routes total, grouped by resource:

| Resource | Routes | Representative paths |
| --- | --- | --- |
| System (status/doctor/containers/prune/master-key/firewall/onboarding/updates) | 12 | `GET /system/status`, `POST /system/prune`, `POST /system/master-key/rotate` |
| Auth/2FA/users/roles/IAM/device-auth/OAuth | 33 | `/auth/login`, `/auth/2fa/*`, `/iam/policies*`, `/auth/device/*` |
| Apps CRUD/lifecycle/deploy | 31 | `/apps`, `/apps/{name}/deploys`, `/restart`, `/exec`, `/deploy-spec`, `/hook-runs` |
| Secrets / git-source / webhooks / previews | 12 | `/apps/{name}/secrets*`, `/webhooks/github/{name}`, `/previews*` |
| Telemetry (metrics/logs) | 10 | `/apps/{name}/metrics`, `/logs/stream`, `/logs/download` |
| Alerts / scheduled tasks / feature flags / notify channels | 22 | `/apps/{name}/alerts`, `/flags/evaluate/{key}`, `/notification-channels*` |
| Databases CRUD + engines + resources | 10 | `/database-engines`, `/databases/{name}/resource-recommendation` |
| Projects / orgs / environments (+ shared env layers) | 20 | `/projects*`, `/organizations/{id}/env` |
| Nodes | 11 | `/nodes`, `/{id}/cordon`, `/drain`, `/workloads` |
| Ingress / certs / domains / email / Cloudflare | 19 | `/certificates`, `/settings/ingress*`, `/settings/cloudflare-tunnel*`, `/domains/{domain}/tls-cert` |
| Static sites / backup targets / registry credentials | 15 | `/static-sites`, `/backup-targets*`, `/registry-credentials*` |
| Git provider apps (GitHub/GitLab/Bitbucket) | 27 | `/github-app*`, `/gitlab-app*`, `/bitbucket-app*` |
| DB backups/restore/clone-restore | 16 | `/databases/{name}/backups*`, `/restore-as-new`, `/backup-schedule` |
| App volume backups/restore | 11 | `/apps/{name}/volumes/{volume}/backups*` |
| App storage/database attach | 5 | `/apps/{name}/storage`, `/apps/{name}/database` |
| Audit log / log-drain | 5 | `/audit-log`, `/audit-log/purge` |

## CLI command groups (`cmd/levelrail-cli/`)

`apps`, `databases`, `auth`, `profile`, `tokens`, `domains`, `backups`,
`app-volume-backups`, `cloudflare-tunnel`, `channels`,
`backup-targets`, `registry-credentials`, `flags`, `nodes`, `status`,
`version`, `audit-log`, `audit-purge`, `doctor`, `containers`,
`firewall`, `users`, `iam`, `secrets`, `migrate`, `completion`.

Key subcommand groups:

- **apps**: create, list, get, deploy, deploy-compose, deploy-spec,
  group, hook-runs, rollback, deploys, promote, restart, stop, start,
  delete, status, diagnose, resource-recommendation, network, logs, exec,
  log-drain, scheduled-tasks, alerts, organizations, projects,
  environments, previews, secrets, git-source, webhook-deliveries
- **databases**: create, list, get, delete, resource-recommendation
- **nodes**: list, get, delete, join-token, cordon, uncordon, drain,
  workloads, health, patch-status
- **iam**: policies create/list/get/update/delete/attach/detach
- **backups** / **app-volume-backups**: list, trigger, restore,
  restore-as-new, schedule, verify, verifications
- **migrate**: coolify, dokploy, caprover

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
