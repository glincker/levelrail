---
title: "Environment variables"
description: Reference for operator-facing APP_ environment variables not covered on feature pages, with defaults, units, and which component reads each one.
"og:url": "https://docs.levelrail.com/environment-variables"
---

This page lists the `APP_*` environment variables you may want to set that no feature page already explains. Each row was checked against the code that reads it, and each default is the fallback in that code. Variables a feature page documents (for example the `APP_INGRESS_*` hardening limits in [Domains and ingress](/domains-and-ingress#edge-limits-client-ips-and-failover) or the `APP_PIPELINE_*` settings in [Pipelines](/pipelines)) stay on those pages. Branding variables (`APP_BRAND_*`) are in [White-labeling](/white-labeling).

## How to set these

Each process reads these variables once at startup, so a change only takes effect after a restart.

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

  This is the same method [Installing](/installing) uses for `APP_ALLOW_INSECURE_LOGIN`.
- **Docker or Compose:** pass them with `-e NAME=value` or an `environment:` block, as in the examples in [Docker](/docker).
- **Node agent:** variables marked "agent" below are read by `levelrail-agent`. Set them on the agent's container or service, not on the control plane. See [Docker](/docker) and [Node provisioning](/node-provisioning).
- **Command line client:** variables marked "CLI" are read by `levelrail-cli` in the shell where you run it.

Duration values use Go duration syntax such as `30s`, `5m`, `1h30m`. Most variables fall back to their default when the value is unset or invalid; the exceptions are called out in the tables.

## Sessions and cookies

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_COOKIE_SECURE` | unset (Secure only when the dashboard URL is `https://`) | Set to `true` to always mark the session cookie Secure. Use it when a proxy terminates TLS in front of an `http://` dashboard URL. Control plane. |

## Supply chain gate

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_SUPPLYCHAIN_FAIL_CLOSED` | `false` | Set to `true` so a blocking gate mode (`block_on_critical`, `block_on_high`) denies a release when the scan is unavailable (recorded as "scan unavailable") and counts unknown severity findings as critical. Control plane. |

## Builds and build cache

The control plane builds images with BuildKit. By default each build starts from whatever cache BuildKit already holds. These variables add a shared cache on top of that. See [Build node routing](/build-node-routing) for dedicated build nodes.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_BUILD_CACHE_DIR` | unset (no local cache backend) | Directory for BuildKit's local cache backend. Builds import from and export to it. Control plane. |
| `APP_BUILD_CACHE_REGISTRY` | unset | Image reference of a registry cache (for example `registry.example.com/cache`) shared across build nodes. When set, it takes priority over the registry settings saved in the dashboard. Control plane. |
| `APP_BUILD_CACHE_REGISTRY_INSECURE` | unset (off) | Set to exactly `true` to allow a plain-HTTP or self-signed cache registry. Only applies when `APP_BUILD_CACHE_REGISTRY` is set. Control plane. |
| `APP_BUILD_TIMEOUT` | `30m` | How long one build may run before it is stopped and marked timed out. A Go duration such as `45m`; `0` means no limit. A value saved in the dashboard or with `apps build-limits set` wins. Control plane. |
| `APP_BUILD_MAX_CONCURRENT` | `2` | Builds allowed to run at once; the rest wait in a queue. `0` means no limit. A saved override wins. Control plane. |
| `APP_BUILD_MEMORY_MB` | `0` | Memory one build may use, in MB. `0` means no limit. A saved global or per-app override wins. Control plane, Linux cgroup v2. |
| `APP_BUILD_CPU_MILLIS` | `0` | CPU one build may use, in thousandths of a core (`1500` is 1.5 cores). `0` means no limit. A saved override wins. Control plane, Linux cgroup v2. |
| `APP_BUILD_CGROUP_ROOT` | `/sys/fs/cgroup` | Where the control plane creates the per-build cgroups that enforce the memory and CPU limits. |
| `APP_REGISTRY_ALLOW_PRIVATE_NETWORKS` | unset (off) | Registries on private LAN ranges (RFC 1918, `fc00::/7`) work by default when you add or browse a registry credential. Set to `true` to also allow loopback registries. Link-local and cloud metadata addresses are always refused. Control plane. |

## Source cache and git fetches

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_SOURCE_CACHE_MAX_BYTES` | `5368709120` | Total size cap of the local git mirror. Control plane. |
| `APP_SOURCE_CACHE_MAX_AGE` | `720h` | Mirrors unused for longer than this are pruned. Control plane. |
| `APP_GIT_FETCH_TIMEOUT` | `10m` | Time limit of one fetch attempt. Control plane. |
| `APP_GIT_FETCH_RETRIES` | `2` | Retries after a network or server error. Control plane. |
| `APP_GIT_FETCH_BACKOFF` | `2s` | First wait between retries, doubled each time. Control plane. |
| `APP_GIT_RECOVERY_INTERVAL` | `30s` | How often a failing git host is probed. Control plane. |
| `APP_GIT_REPLAY_WINDOW` | `24h` | How old a push may be and still be replayed once the host recovers. Control plane. |

See [Deploying through git host outages](/git-host-outages).

## Git webhook (single repository mode)

These variables turn on the control plane's own `POST /webhook` receiver for a single repository. They are separate from the per-app webhooks and GitHub App flow in [Git integrations](/git-integrations). The three required variables must all be set. If any is missing, the control plane still starts and logs a `webhook not configured` warning, and only this receiver is unavailable.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_GIT_REPO_URL` | none, required | Git remote the receiver clones when a push arrives. Control plane. |
| `APP_WEBHOOK_SECRET` | none, required | Shared secret used to verify the `X-Hub-Signature-256` header on incoming pushes. Control plane. |
| `APP_IMAGE_REPO` | none, required | Image repository that built images are tagged into. Control plane. |
| `APP_GIT_BRANCH` | `main` | Only pushes to this branch trigger a deploy. Control plane. |
| `APP_SPEC_DIR` | `.` (the process working directory) | Directory searched for the app spec file. See [App spec reference](/app-spec-reference). Control plane. |
| `APP_SERVICE_NAME` | the only service in the spec | Which service from the spec to deploy. Required when the spec declares more than one service. Control plane. |

## GitHub App manifest

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_GITHUB_APP_MANIFEST_FILE` | `./deploy/github-app-manifest.yaml` | YAML file with `default_permissions` (a map) and `default_events` (a list) that a newly registered GitHub App requests. A missing file means the built-in set: `contents:read`, `metadata:read`, `repository_hooks:write`, `statuses:write`, `deployments:write`, and no events. A file that exists but cannot be parsed stops the load with an error. The relative default resolves against the process working directory, which is the data directory under the installer's unit. Control plane. |

## Pipelines and OIDC

The pipeline engine's own tuning variables are listed in [Pipelines](/pipelines). These extra ones cover the scheduler, commit status reporting, and the OIDC token service described in [Pipelines OIDC](/pipelines-oidc). Non-positive or unparseable values for the duration variables here fall back to the default without a warning.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_PIPELINE_SCHEDULER_INTERVAL` | `30s` | How often the pipeline scheduler runs. Control plane. |
| `APP_GIT_STATUS_TIMEOUT` | `10s` | Time limit for each commit status post a pipeline run sends to the git forge. Control plane. |
| `APP_OIDC_KEY_RETIRE_GRACE` | `24h` | How long a rotated-out signing key stays published in the JWKS document before it is removed. Control plane. |
| `APP_OIDC_JWKS_RATE_LIMIT_PER_MINUTE` | `60` | Rate limit on the unauthenticated JWKS endpoint. Positive integer. Control plane. |
| `APP_OIDC_TOKEN_REQUEST_PORT` | `9095` | Port of the runtime token request endpoint, bound to the Docker bridge gateway address so only job containers can reach it. Positive integer. Control plane. |

## Scheduling and background intervals

Each of these sets how often a background loop wakes to check for work. Five-field cron only goes down to one minute, so the one-minute defaults are as short as the two cron-driven schedulers can usefully go.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_BACKUP_SCHEDULER_INTERVAL` | `1m` | How often the database backup scheduler checks for backups whose cron schedule is due. See [Backups and storage](/backups-and-storage). Control plane. |
| `APP_SCHEDULED_TASK_SCHEDULER_INTERVAL` | `1m` | How often the scheduler checks for scheduled tasks that are due. Control plane. |
| `APP_DEPLOY_APPROVAL_SWEEP_INTERVAL` | `5m` | How often pending deploy approvals are checked against their 24 hour expiry. See [Chat deploy approvals](/chat-deploy-approvals). Control plane. |
| `APP_RESOURCE_RECOMMENDATION_LOOKBACK` | `168h` (7 days) | History window used when computing app resource recommendations. Control plane. |
| `APP_RECONCILE_WORKERS` | `1` | How many apps may reconcile at once, so one slow or failing app does not delay the others. `1` keeps the sequential pass; `4` or `8` is recommended with many apps. Above 1, each app reconcile is cut after `APP_RECONCILE_KEY_TIMEOUT` (default `10m`) and retried. Control plane. |
| `APP_RECONCILE_READINESS_REQUEUE` | `2s` | Go duration. Delay before a not-yet-ready app is checked again. Only used when `APP_RECONCILE_WORKERS` is above 1. Control plane. |

For the invalid-value behavior of the first three, see the note at the end of this page.

## Control plane off-box backups

These tune the scheduled off-box copy of the control plane's own state and its restore drills, described in [Control plane backup](/control-plane-backup) and [Disaster recovery](/disaster-recovery). Values that are unset, unparseable, or not greater than zero use the default. Control plane.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_CONTROL_PLANE_OFFBOX_RETAIN_WEEKLY` | `4` | Number of weekly backups kept. Positive integer. |
| `APP_CONTROL_PLANE_OFFBOX_RETAIN_MONTHLY` | `6` | Number of monthly backups kept. Positive integer. |

## Point-in-time restore

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_PITR_WAL_SEGMENT_BYTES` | `16777216` (16 MiB) | WAL segment size, in bytes, that the restore path assumes when it maps a base backup's start position to a segment. Set it only if the Postgres server was initialized with a non-default WAL segment size. Positive integer. Control plane. See [Managing databases](/managing-databases#point-in-time-restore). |
| `APP_PITR_BINLOG_EXPIRE_SECONDS` | `604800` (7 days) | How long a MySQL or MariaDB database keeps closed binary log files on its own host. Shipped copies in the backup target are pruned by base backup retention instead. Positive integer. Control plane. |
| `APP_PITR_MYSQL_TOOLS_IMAGE` | `percona/percona-server:8.4` | Image a MySQL point-in-time restore starts briefly to run `mysqlbinlog`, which the official MySQL image does not ship. MariaDB restores do not use it. Control plane. |
| `APP_PREVIEW_BUILD_SECRETS` | unset (off) | Set to a true value to let preview (pull request) builds use `build.secrets`. Off by default because a pull request can change the Dockerfile and read the secret. Read on every build. Control plane. |

## Log archive

The archiver's other settings are on [Log archive](/log-archive). These two bound how much one run does. Positive values only; anything else uses the default. Control plane.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_LOG_ARCHIVE_MAX_WINDOWS_PER_RUN` | `24` | Maximum number of one-hour windows archived in a single scheduled run. Integer. |
| `APP_LOG_ARCHIVE_MAX_BACKFILL` | `24h` | How far back a run may reach when it catches up on missed windows. Duration. |

## Backup copies

See [Backup copies](/backup-copies). Control plane.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_BACKUP_COPY_LAG_ALERT` | `1h` | Time a backup may go without a second copy before `backup_copy_behind` fires. Duration. |
| `APP_BACKUP_COPY_CONCURRENCY` | `2` | Copies running at once. Positive integer. |

## Data export

How these are used is on [Data export](/data-export). Positive values only; anything else uses the default. Control plane.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_EXPORT_ALLOW_PRIVATE_NETWORKS` | `false` | Let export destinations resolve to private, loopback or link-local addresses, for a Loki or collector on the same host or network. Boolean. |
| `APP_EXPORT_RESYNC` | `15s` | How often running exporters are reconciled with the configured destinations. Duration. |
| `APP_EXPORT_QUEUE_LINES` | `10000` | In-memory queue length per log destination. The oldest entries are dropped and counted when it is full. Integer. |
| `APP_EXPORT_BATCH_LINES` | `200` | Maximum entries per delivered batch. Integer. |
| `APP_EXPORT_BATCH_BYTES` | `1048576` | Approximate maximum batch size in bytes. Integer. |
| `APP_EXPORT_FLUSH_INTERVAL` | `2s` | How long a partial batch waits for more entries. Duration. |
| `APP_EXPORT_MAX_ATTEMPTS` | `4` | Delivery attempts per batch before it is dropped (audit batches are held instead). Integer. |
| `APP_EXPORT_BACKOFF_BASE` | `500ms` | First retry delay, doubled per attempt with jitter. Duration. |
| `APP_EXPORT_BACKOFF_MAX` | `15s` | Upper bound for the retry delay. Duration. |
| `APP_EXPORT_BREAKER_FAILURES` | `5` | Consecutive failed batches that open a destination's circuit breaker. Integer. |
| `APP_EXPORT_BREAKER_COOLDOWN` | `60s` | How long an open breaker waits before one trial batch. Duration. |
| `APP_EXPORT_SEND_TIMEOUT` | `10s` | Timeout of one delivery request. Duration. |
| `APP_EXPORT_STATUS_INTERVAL` | `5s` | How often delivery counters are saved. Duration. |
| `APP_EXPORT_AUDIT_INTERVAL` | `10s` | How often the audit log is polled for new rows. Duration. |
| `APP_EXPORT_AUDIT_BATCH` | `500` | Audit rows per batch and per page of a download. Integer. |
| `APP_EXPORT_AUDIT_MAX_ROWS` | `1000000` | Maximum rows in one audit log download. Integer. |
| `APP_EXPORT_METRICS_INTERVAL` | `60s` | Default OTLP metrics push interval. Duration. |

## Ingress

The edge hardening limits are in [Domains and ingress](/domains-and-ingress#edge-limits-client-ips-and-failover). These two are not listed there. Control plane.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_INGRESS_RETRY_INTERVAL` | `250ms` | Pause between retries when a proxied backend cannot be reached during the `APP_INGRESS_RETRY_WINDOW`. A negative or unparseable value is a startup error, not a silent fallback. |
| `APP_INGRESS_SOCKET_ACTIVATION` | `auto` | Controls use of sockets inherited from systemd. `auto` uses them if present. `true` (also `1`, `on`) makes missing inherited sockets a startup error. `false` (also `0`, `off`) ignores them. See [Surviving a control plane restart](/domains-and-ingress#surviving-a-control-plane-restart). |

## Certificates and request thresholds

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_CERT_EXPIRY_WARNING_WINDOW` | `336h` (14 days) | How close to expiry a certificate must be before it counts as expiring, for both the certificate expiry alert rules and the API. Control plane. |
| `APP_CERT_FAIL_DAYS` | `14` | Days left below which an auto-renewed certificate makes the doctor's certificate check fail (and raises an attention item). Control plane. |
| `APP_CERT_WATCHDOG_INTERVAL` | `10m` | How often the renewal watchdog looks for stuck or failing renewals. Control plane. |
| `APP_CERT_WATCHDOG_BACKOFF` | `30m` | First retry delay of the renewal watchdog; it doubles per attempt up to 12 hours. Control plane. |
| `APP_ACME_FAILED_PER_HOUR` | `5` | Failed validations per hostname per hour the certificate preflight treats as the Let's Encrypt limit. Control plane. |
| `APP_ACME_DUPLICATE_PER_WEEK` | `5` | Certificates per exact name per week the preflight treats as the Let's Encrypt duplicate limit. Control plane. |
| `APP_SLOW_REQUEST_THRESHOLD` | `500ms` | API requests slower than this are logged at Warn level, with a `breakdown` field naming the slowest phases (for example `telemetry_latest=1.2s`). Control plane. |
| `APP_CRITICAL_REQUEST_THRESHOLD` | `2s` | API requests slower than this are logged at Error level. Control plane. |
| `APP_DASHBOARD_CACHE_TTL` | `5s` | How long the dashboard's aggregate reads (app and node resource usage, apps metrics, system status, doctor) are reused between requests. `0` disables the cache. Control plane. |
| `APP_DASHBOARD_CACHE_STALE` | `1m` | How long past the TTL a cached aggregate may still be served while one background refresh runs. Control plane. |

For all of these, a value that does not parse logs a warning and uses the default. For the two request thresholds, zero or a negative duration also means the default.

## Doctor and diagnosis

These tune `levelrail-cli doctor` and the structured diagnosis of failed deploys. See [Troubleshooting](/troubleshooting) and [Deploy failures](/deploy-failures).

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_DOCTOR_DISK_WARNING_BYTES` | `1073741824` (1 GiB) | Free disk space below which the `disk_space` check warns. Integer bytes. Control plane. |
| `APP_DOCTOR_MASTER_KEY_ROTATION_WARN_DAYS` | `365` | Age of the master key, in whole days, after which doctor suggests rotating it. See [Master key rotation](/master-key-rotation). Control plane. |
| `APP_DOCTOR_NETWORK_TIMEOUT` | `5s` | Shared time limit for the network checks in the doctor endpoint. Control plane. |
| `APP_PROXY_ROUTER_PRIORITY` | `100000` | Traefik router priority written into managed route files, 1 to 1000000000. High so the managed router wins over label routers (for example ones left over from Coolify) for the same host. Changing it rewrites every managed file, so run **Verify** again afterwards. Control plane. |
| `APP_TAKEOVER_WINDOW` / `APP_TAKEOVER_VERIFY_INTERVAL` | `5m` / `10s` | How long a same-server port takeover may stay unverified before it rolls back by itself, and the pause between checks. Control plane. See [Migrate on the same server](/migrate-same-server). |
| `APP_TAKEOVER_MIN_DISK_GB` / `APP_TAKEOVER_MIN_MEMORY_MB` | `2` / `256` | Free disk (GB) and total memory (MB) below which the takeover preflight blocks. Control plane. |
| `APP_PROXY_VERIFY_TIMEOUT` | `8s` | Time limit for each managed proxy route check (TLS handshake and request through the proxy) and for the DNS checks of the proxy setup checklist. Control plane. |
| `APP_DOCTOR_PUBLIC_IP_ENDPOINT` | `https://api.ipify.org` | URL that returns the caller's public IP as plain text, used by the `public_ip` doctor check and by deploy preflight. Control plane. |
| `APP_DIAGNOSE_PROBE_TIMEOUT` | `4s` | Time limit for each live probe (container state and container logs) when diagnosing an app. Zero or invalid values use the default. Control plane. |
| `APP_ATTENTION_COPY_FAILURE_DAYS` | `14` | Days a failed data copy stays in the attention feed before it drops to history. Positive integer. Control plane. |
| `APP_ATTENTION_DIAGNOSE_LIMIT` | `10` | Maximum number of distinct apps `levelrail-cli attention` asks the control plane to diagnose when it marks items as fixable. Integer, `0` skips diagnosis. CLI. |

## Updates

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_UPDATE_CHECK_INTERVAL` | `1h` | How often the control plane checks the configured release channel for a newer version. Control plane. |

## SSH node provisioning

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_SSH_NODE_PROVISION_TIMEOUT` | `10m` | Time limit for installing the agent over SSH, and how long the control plane waits for the new node to enroll before reporting the provision as failed. Zero or invalid values use the default. Control plane. See [Node provisioning](/node-provisioning). |

## Mesh networking

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_MESH_KEY_FILE` | `levelrail-agent-mesh.key` in the same directory as the agent identity file | Path where the agent stores its WireGuard private key. Only used when `APP_MESH_ENABLED=1`. Agent. See [Network topology](/network-topology). |

## App networks

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_NETWORK_POOL` | `10.88.0.0/14` | IPv4 block per-app Docker networks get explicit /24 subnets from, about 1000 apps. Must not overlap host interfaces, routes or other Docker networks (already-used ranges are skipped). `off` uses the Docker daemon's own pools (about 30 networks). Existing networks are never re-addressed. Control plane. See [Troubleshooting](/troubleshooting#app-network-pool). |
| `APP_NETWORK_GC_GRACE` | `10m` | How long a network whose app is gone must sit empty before it is removed. Control plane. |

## Secret rotation

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_SECRET_ROTATION_INTERVAL_SECONDS` | `3600` | How often the rotation sweep runs, in seconds. Positive integer. Control plane. See [Secret rotation policies](secret-rotation.md). |
| `APP_SECRET_ROTATION_DUE_SOON_DAYS` | `7` | A policy shows as due soon this many days before its due date. Positive integer. Control plane. |
| `APP_SECRET_ROTATION_MAX_PER_PASS` | `10` | Most scheduled rotations one sweep starts, so restarts spread out. Positive integer. Control plane. |
| `APP_SECRET_ROTATION_GRACE_HOURS` | `24` | How long a manually rotated app secret or Vault credential keeps its previous value. Positive integer of hours. Control plane. |
| `APP_SECRET_ROTATION_DEFAULT_INTERVAL_DAYS` | `90` | Rotation interval given to a secret that is rotated by hand before it has a policy. Positive integer. Control plane. |

## Behavior to know about

- **Scheduler intervals are not range checked.** `APP_BACKUP_SCHEDULER_INTERVAL`, `APP_SCHEDULED_TASK_SCHEDULER_INTERVAL`, `APP_UPDATE_CHECK_INTERVAL` and `APP_DEPLOY_APPROVAL_SWEEP_INTERVAL` use any value that parses as a Go duration, including zero or a negative one. Only an unparseable value falls back to the default. Use a positive duration.
- **Ingress variables fail closed.** Unlike most settings on this page, an invalid `APP_INGRESS_*` hardening value is returned as an error at startup rather than ignored.

## Troubleshooting

### Variable change did not take effect

**Likely cause:** The process was not restarted after the variable was set.

**How to confirm:** Check the current setting in the dashboard or via the CLI. Run `systemctl status levelrail` to confirm the service is running.

**Fix:** Restart the service or process: `sudo systemctl restart levelrail` on the control plane, or `docker restart <container-name>` if running in Docker.

### Invalid value for a duration variable

**Likely cause:** The duration syntax is incorrect. Go duration syntax requires a unit: `30s`, `5m`, `1h30m`.

**How to confirm:** Check the control plane logs for a startup error mentioning the variable: `journalctl -u levelrail` or `docker logs <container-name>`.

**Fix:** Use valid Go duration syntax with units (s, m, h). For example, `APP_BACKUP_SCHEDULER_INTERVAL=30m` instead of `APP_BACKUP_SCHEDULER_INTERVAL=30`.

### Ingress variable causes startup failure

**Likely cause:** An invalid `APP_INGRESS_*` variable was set. Ingress variables fail closed and refuse to start if the value is invalid.

**How to confirm:** Check the control plane logs. The error will mention which ingress variable is invalid.

**Fix:** Review the variable value and correct the syntax. Restart the service once the variable is fixed.

### Still stuck

For help with specific variables, refer to the table for that variable's scope (Control plane, Agent, or CLI) and component. Variables are only read at startup, so restart the relevant service after making changes.

## Volume files

See [Volume files](/volume-files). Control plane and agent.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_VOLUME_FILE_MAX_UPLOAD_MB` | `64` | Largest file accepted by a volume file upload or single-file restore. Positive integer. |
| `APP_VOLUME_FILE_LIST_LIMIT` | `1000` | Most entries returned for one volume directory. Positive integer. |
