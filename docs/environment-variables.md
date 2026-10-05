---
description: Reference for operator-facing APP_ environment variables not covered on feature pages, with defaults, units, and which component reads each one.
---

# Environment variables

This page lists the operator-facing `APP_*` environment variables that are not already explained on a feature page. Each row was checked against the code that reads it, and each default is the fallback in that code. Variables that a feature page already documents (for example the `APP_INGRESS_*` hardening limits in [Domains and ingress](domains-and-ingress.md#edge-limits-client-ips-and-failover) or the `APP_PIPELINE_*` settings in [Pipelines](pipelines.md)) stay on those pages. Branding variables (`APP_BRAND_*`) are covered in [White-labeling](white-labeling.md).

<InlineToc default-open />

## How to set these

Every variable on this page is read by a process at startup, so a change takes effect after a restart.

- **Installed with `install.sh`:** the control plane runs as the `levelrail` systemd service. The unit file sets only `APP_DATA_DIR`, `APP_HTTP_ADDR`, `APP_INGRESS_HTTP_ADDR` and `APP_INGRESS_HTTPS_ADDR` itself, with plain `Environment=` lines and no `EnvironmentFile=`. Add your own with a drop-in, which survives upgrades:

  ```bash
  sudo systemctl edit levelrail
  ```

  Then add the variables under a `[Service]` heading and restart:

  ```ini
  [Service]
  Environment=APP_UPDATE_CHECK_INTERVAL=6h
  Environment=APP_SLOW_REQUEST_THRESHOLD=1s
  ```

  ```bash
  sudo systemctl restart levelrail
  ```

  This is the same method [Installing](installing.md) uses for `APP_ALLOW_INSECURE_LOGIN`.
- **Docker or Compose:** pass them with `-e NAME=value` or an `environment:` block, as in the examples in [Docker](docker.md).
- **Node agent:** variables marked "agent" below are read by `levelrail-agent`. Set them on the agent's container or service, not on the control plane. See [Docker](docker.md) and [Node provisioning](node-provisioning.md).
- **Command line client:** variables marked "CLI" are read by `levelrail-cli` in the shell where you run it.

Duration values use Go duration syntax such as `30s`, `5m`, `1h30m`. Most variables fall back to their default when the value is unset or invalid; the exceptions are called out in the tables.

## Builds and build cache

The control plane builds images with BuildKit. By default each build starts from whatever cache BuildKit already holds. These variables add a shared cache. See [Build node routing](build-node-routing.md) for dedicated build nodes.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_BUILD_CACHE_DIR` | unset (no local cache backend) | Directory for BuildKit's local cache backend. Builds import from and export to it. Control plane. |
| `APP_BUILD_CACHE_REGISTRY` | unset | Image reference of a registry cache (for example `registry.example.com/cache`) shared across build nodes. When set, it takes priority over the registry settings saved in the dashboard. Control plane. |
| `APP_BUILD_CACHE_REGISTRY_INSECURE` | unset (off) | Set to exactly `true` to allow a plain-HTTP or self-signed cache registry. Only applies when `APP_BUILD_CACHE_REGISTRY` is set. Control plane. |

## Git webhook (single repository mode)

These variables turn on the control plane's own `POST /webhook` receiver for one repository. They are separate from the per-app webhooks and GitHub App flow in [Git integrations](git-integrations.md). The three required variables must all be set. If any is missing, the control plane still starts and logs a `webhook not configured` warning, and only this receiver is unavailable.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_GIT_REPO_URL` | none, required | Git remote the receiver clones when a push arrives. Control plane. |
| `APP_WEBHOOK_SECRET` | none, required | Shared secret used to verify the `X-Hub-Signature-256` header on incoming pushes. Control plane. |
| `APP_IMAGE_REPO` | none, required | Image repository that built images are tagged into. Control plane. |
| `APP_GIT_BRANCH` | `main` | Only pushes to this branch trigger a deploy. Control plane. |
| `APP_SPEC_DIR` | `.` (the process working directory) | Directory searched for the app spec file. See [App spec reference](app-spec-reference.md). Control plane. |
| `APP_SERVICE_NAME` | the only service in the spec | Which service from the spec to deploy. Required when the spec declares more than one service. Control plane. |

## GitHub App manifest

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_GITHUB_APP_MANIFEST_FILE` | `./github-app-manifest.yaml` | YAML file with `default_permissions` (a map) and `default_events` (a list) that a newly registered GitHub App requests. A missing file means the built-in set: `contents:read`, `metadata:read`, `repository_hooks:write`, `statuses:write`, `deployments:write`, and no events. A file that exists but cannot be parsed stops the load with an error. The relative default resolves against the process working directory, which is the data directory under the installer's unit. Control plane. |

## Pipelines and OIDC

The pipeline engine's own tuning variables are listed in [Pipelines](pipelines.md). These extra ones cover the scheduler, commit status reporting, and the OIDC token service described in [Pipelines OIDC](pipelines-oidc.md). Non-positive or unparseable values for the duration variables here fall back to the default without a warning.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_PIPELINE_SCHEDULER_INTERVAL` | `30s` | How often the pipeline scheduler runs. Control plane. |
| `APP_GIT_STATUS_TIMEOUT` | `10s` | Time limit for each commit status post a pipeline run sends to the git forge. Control plane. |
| `APP_OIDC_KEY_RETIRE_GRACE` | `24h` | How long a rotated-out signing key stays published in the JWKS document before it is removed. Control plane. |
| `APP_OIDC_JWKS_RATE_LIMIT_PER_MINUTE` | `60` | Rate limit on the unauthenticated JWKS endpoint. Positive integer. Control plane. |
| `APP_OIDC_TOKEN_REQUEST_PORT` | `9095` | Port of the runtime token request endpoint, bound to the Docker bridge gateway address so only job containers can reach it. Positive integer. Control plane. |

## Scheduling and background intervals

Each of these is how often a background loop wakes to check for work. Standard five-field cron has one-minute granularity, so the one-minute defaults are the shortest useful value for the two cron-driven schedulers.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_BACKUP_SCHEDULER_INTERVAL` | `1m` | How often the database backup scheduler checks for backups whose cron schedule is due. See [Backups and storage](backups-and-storage.md). Control plane. |
| `APP_SCHEDULED_TASK_SCHEDULER_INTERVAL` | `1m` | How often the scheduler checks for scheduled tasks that are due. Control plane. |
| `APP_DEPLOY_APPROVAL_SWEEP_INTERVAL` | `5m` | How often pending deploy approvals are checked against their 24 hour expiry. See [Chat deploy approvals](chat-deploy-approvals.md). Control plane. |
| `APP_RESOURCE_RECOMMENDATION_LOOKBACK` | `168h` (7 days) | History window used when computing app resource recommendations. Control plane. |

For the invalid-value behavior of the first three, see the note at the end of this page.

## Control plane off-box backups

These tune the scheduled off-box copy of the control plane's own state and its restore drills, described in [Control plane backup](control-plane-backup.md) and [Disaster recovery](disaster-recovery.md). Values that are unset, unparseable, or not greater than zero use the default. Control plane.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_CONTROL_PLANE_OFFBOX_RETAIN_WEEKLY` | `4` | Number of weekly backups kept. Positive integer. |
| `APP_CONTROL_PLANE_OFFBOX_RETAIN_MONTHLY` | `6` | Number of monthly backups kept. Positive integer. |

## Point-in-time restore

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_PITR_WAL_SEGMENT_BYTES` | `16777216` (16 MiB) | WAL segment size, in bytes, that the restore path assumes when it maps a base backup's start position to a segment. Set it only if the Postgres server was initialized with a non-default WAL segment size. Positive integer. Control plane. See [Managing databases](managing-databases.md#point-in-time-restore-postgres-only). |

## Log archive

The archiver's other settings are on [Log archive](log-archive.md). These two bound how much one run does. Positive values only; anything else uses the default. Control plane.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_LOG_ARCHIVE_MAX_WINDOWS_PER_RUN` | `24` | Maximum number of one-hour windows archived in a single scheduled run. Integer. |
| `APP_LOG_ARCHIVE_MAX_BACKFILL` | `24h` | How far back a run may reach when it catches up on missed windows. Duration. |

## Ingress

The edge hardening limits are in [Domains and ingress](domains-and-ingress.md#edge-limits-client-ips-and-failover). These two are not listed there. Control plane.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_INGRESS_RETRY_INTERVAL` | `250ms` | Pause between retries when a proxied backend cannot be reached during the `APP_INGRESS_RETRY_WINDOW`. A negative or unparseable value is a startup error, not a silent fallback. |
| `APP_INGRESS_SOCKET_ACTIVATION` | `auto` | Controls use of sockets inherited from systemd. `auto` uses them if present. `true` (also `1`, `on`) makes missing inherited sockets a startup error. `false` (also `0`, `off`) ignores them. See [Surviving a control plane restart](domains-and-ingress.md#surviving-a-control-plane-restart). |

## Certificates and request thresholds

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_CERT_EXPIRY_WARNING_WINDOW` | `336h` (14 days) | How close to expiry a certificate must be before it counts as expiring, for both the certificate expiry alert rules and the API. Control plane. |
| `APP_SLOW_REQUEST_THRESHOLD` | `500ms` | API requests slower than this are logged at Warn level. Control plane. |
| `APP_CRITICAL_REQUEST_THRESHOLD` | `2s` | API requests slower than this are logged at Error level. Control plane. |

For all three, a value that does not parse logs a warning and uses the default. For the two request thresholds, zero or a negative duration also means the default.

## Doctor and diagnosis

These tune `levelrail-cli doctor` and the structured diagnosis of failed deploys. See [Troubleshooting](troubleshooting.md) and [Deploy failures](deploy-failures.md).

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_DOCTOR_DISK_WARNING_BYTES` | `1073741824` (1 GiB) | Free disk space below which the `disk_space` check warns. Integer bytes. Control plane. |
| `APP_DOCTOR_MASTER_KEY_ROTATION_WARN_DAYS` | `365` | Age of the master key, in whole days, after which doctor suggests rotating it. See [Master key rotation](master-key-rotation.md). Control plane. |
| `APP_DOCTOR_NETWORK_TIMEOUT` | `5s` | Shared time limit for the network checks in the doctor endpoint. Control plane. |
| `APP_DOCTOR_PUBLIC_IP_ENDPOINT` | `https://api.ipify.org` | URL that returns the caller's public IP as plain text, used by the `public_ip` doctor check and by deploy preflight. Control plane. |
| `APP_DIAGNOSE_PROBE_TIMEOUT` | `4s` | Time limit for each live probe (container state and container logs) when diagnosing an app. Zero or invalid values use the default. Control plane. |
| `APP_ATTENTION_DIAGNOSE_LIMIT` | `10` | Maximum number of distinct apps `levelrail-cli attention` asks the control plane to diagnose when it marks items as fixable. Integer, `0` skips diagnosis. CLI. |

## Updates

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_UPDATE_CHECK_INTERVAL` | `1h` | How often the control plane checks the configured release channel for a newer version. Control plane. |

## SSH node provisioning

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_SSH_NODE_PROVISION_TIMEOUT` | `10m` | Time limit for installing the agent over SSH, and how long the control plane waits for the new node to enroll before reporting the provision as failed. Zero or invalid values use the default. Control plane. See [Node provisioning](node-provisioning.md). |

## Mesh networking

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_MESH_KEY_FILE` | `levelrail-agent-mesh.key` in the same directory as the agent identity file | Path where the agent stores its WireGuard private key. Only used when `APP_MESH_ENABLED=1`. Agent. See [Network topology](network-topology.md). |

## Behavior to know about

- **Scheduler intervals are not range checked.** `APP_BACKUP_SCHEDULER_INTERVAL`, `APP_SCHEDULED_TASK_SCHEDULER_INTERVAL`, `APP_UPDATE_CHECK_INTERVAL` and `APP_DEPLOY_APPROVAL_SWEEP_INTERVAL` use any value that parses as a Go duration, including zero or a negative one. Only an unparseable value falls back to the default. Use a positive duration.
- **Ingress variables fail closed.** Unlike most settings on this page, an invalid `APP_INGRESS_*` hardening value is returned as an error at startup rather than ignored.
