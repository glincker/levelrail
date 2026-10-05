---
description: Inventory of Levelrail's dashboard pages by area, with pointers to the generated API and CLI references.
---

# Feature catalog

A flat map of what the dashboard offers, so you can find the right page or check whether something already exists. This is a hand-written snapshot. The API and CLI surfaces change faster than this page can, so they live in generated or per-command references instead: [API reference](api-reference.md) (generated from the registered routes) and [CLI reference](cli-reference.md). For how mature each feature is, see [feature status](feature-status.md). For history, see the [roadmap](roadmap.md).

## Dashboard pages

What you can see and do for each app, database, and system area.

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
| Integrations | Send app logs to external log drains |
| Logs | Search and filter logs from running containers with full-text search, or tail live logs in real time |
| Metrics | View CPU, memory, disk I/O, network I/O, request rate, response times, error rate, and container restart counts over time |
| Network | See which node the app is running on and internal DNS names for communicating with other apps and databases |
| Resources | Set CPU and memory limits so the app does not starve others or consume unbounded resources; see platform recommendations based on actual usage |
| Scheduled tasks | Set up cron-like tasks that run inside the app on a schedule |
| Services | Manage multi-service apps: a web frontend plus a background worker, both under the same app.yaml |
| Source | Connect a git repository and configure branch-to-environment mapping, preview environments per pull request, and inspect webhook deliveries from your git provider |
| Volumes | Back up app storage volumes to S3, restore from a backup, or browse backup history |
| Alerts | Set up alerts that fire when metrics cross a threshold, and choose notification channels |
| Load balancer | Multi-replica routing and health history (behind `APP_EXPERIMENTAL=load-balancer`) |
| Pipelines | Build and test pipelines attached to the app, see [pipelines](pipelines.md) |

### Databases (`/databases/$name/*`)

| Page | What you can do |
| --- | --- |
| Overview | Manage backups and restore points, expose the database outside the Docker network for external tools, attach it to apps, and check TLS certificate status |
| Logs | View detailed activity logs from the database engine |
| Slow queries | Long-running queries surfaced from the engine's slow query log (Postgres and MySQL) |
| Metrics | Monitor resource usage over time: CPU, memory, network I/O, and disk I/O |
| Resources | Set CPU and memory limits and see platform recommendations based on usage |

### Top-level pages

| Page | What you can do |
| --- | --- |
| Apps, Databases | List, search and create apps and databases, including from [templates](templates-and-registry.md) |
| Deployments | Fleet-wide deploy history, see [deployments page](deployments-page.md) |
| Approvals | Review pending deploy approvals, see [deploy safety](deploy-safety.md) |
| Backups | Instance-wide backup list across databases and volumes, see [backups and storage](backups-and-storage.md) |
| Alerts | Alert rules and their state |
| Domains | Cross-app domain list with TLS status |
| Network | Network view across nodes, see [network topology](network-topology.md) |
| Nodes | List managed servers, check health, cordon, drain and view node metrics |
| Projects | Group apps and databases by project. Each project has a [Topology](service-topology-graph.md) page drawing its apps, databases and shared volumes |
| Organizations | Multi-tenant grouping for teams or business units |
| Pipelines | Pipelines across apps |
| Load balancers | Overview across apps (needs `load-balancer` enabled) |
| Models | AI models on GPU nodes (needs `ai-models` enabled) |
| AI assistant | In-app chat (needs `ai-chat` enabled), see [AI assistant chat](ai-assistant-chat.md) |
| Status | Everything that needs attention: failing apps, offline nodes, certificates, doctor findings |

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
| Notification channels | Set up the 18 channel kinds for alert notifications, see [integrations](integrations.md) |
| Organizations | Manage organization-level settings |
| OAuth sign-in | Enable sign-in via Google, GitHub, Microsoft or a generic OpenID Connect provider |
| Git provider apps | Connect GitHub Apps, GitLab integrations, and Bitbucket connections for automatic deployments |
| Cloudflare Tunnel | Route traffic through Cloudflare instead of opening ports directly |
| Vault | Connect HashiCorp Vault for secret storage |
| Email | Configure outgoing email for invites and notifications |
| System status | Run the doctor diagnostic, view the status page, and check the attention panel for critical issues |
| Containers | View all containers running on the control plane |
| Firewall | View and sync host firewall rules |
| Control plane backup | Snapshot, off-box backup and restore drills for the control plane, see [control-plane backup](control-plane-backup.md) |
| Storage | Object storage destinations, see [object storage](object-storage.md) |
| Network shares | Shared network storage definitions |
| Node providers | Cloud provider credentials for provisioning nodes, see [node provisioning](node-provisioning.md) |
| Observability | Retention and collection settings, see [observability](observability.md) |
| Status page | Public status page settings, see [status page](status-page.md) |
| Import platform | Import apps from another platform, see [importing apps](importing-apps.md) |
| API explorer | Try API calls from the dashboard, see [API explorer](api-explorer.md) |
| Updates | Check and install platform updates |

## API and CLI

- The [API reference](api-reference.md) lists every registered route with its method, path and required ability, grouped by resource. It is generated by `go run ./scripts/gen-api-reference` and a test fails if a route is missing.
- The [CLI reference](cli-reference.md) lists every `levelrail-cli` command group with flags and examples. Run `levelrail-cli --help` for the groups your build and `APP_EXPERIMENTAL` setting expose.
- Connecting a git provider App or OAuth integration stays in the dashboard (it needs a browser redirect through the provider). The CLI can browse and use an already-connected integration's repositories.

## See also

- [Feature status](feature-status.md): maturity labels and evidence.
- [Roadmap](roadmap.md): what has shipped and what is open.
- [Getting started](getting-started.md): your first deploy.
