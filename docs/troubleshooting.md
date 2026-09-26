---
description: Fixes for the most common problems when deploying, logging in, or running Levelrail.
---

# Troubleshooting

Start here for a fast fix. Each entry links to the full page if you need more depth.

::: details My deploy is stuck or failed
1. Check the build log first: dashboard's deploy detail page, or `levelrail-cli apps deploys logs <name> <deploy-id>`.
2. If the build succeeded but the app never came up, the readiness probe is the usual cause. The `Ready` condition's message names the exact request or command and what came back (a status, a redirect target, a TLS error, an exec exit code). A 302 to a login page wants `follow_redirects` or `expected_status: 200-399`; a self-signed HTTPS endpoint wants `scheme: https` with `tls_skip_verify: true`; a database is better checked with an `exec` probe. See [Health checks](app-spec-reference.md#health-checks). A slow cold start (JVM warm-up, a large migration) can also legitimately take longer than the default 60s readiness budget: raise it with `health.readyTimeout` instead of treating the false `ReadinessFailed` as a real bug.
3. A crashlooping container gets its last 200 log lines surfaced automatically in the dashboard, no separate log search needed.
4. If this keeps happening on every deploy of a given app, consider turning on "Auto-rollback on crashloop" (Deploys tab, off by default) so the next crashloop redeploys the previous known-good image automatically instead of retrying the same bad one. See [Observability](observability.md).
5. Still stuck: [Deploying apps](deploying-apps.md#health-checks) covers the full health check contract.
:::

::: details Something is wrong but I don't know what
Open the Status page (`/status`, with a badge in the sidebar) or run `levelrail-cli attention`. Both list failing apps, offline nodes, expired or expiring certificates, and doctor warnings or failures, critical first. The CLI exits 1 when any item is critical. A node listed as offline has a connection history: `levelrail-cli nodes events <id>`.
:::

::: details TLS certificate won't issue
This has its own dedicated runbook: [ACME verification runbook](acme-verification-runbook.md). Start there; it covers DNS propagation, rate limits, and staging-vs-production ACME directories.

To see whether renewal is failing, run `levelrail-cli domains certificates`. The `RENEWAL` column is `ok` or `stalled`, and the same value is the `renewal` field of `GET /api/v1/certificates`. The dashboard shows a "Renewal stalled" badge on the domain row and in the domain editor. A certificate is `stalled` when it has already expired, or when it has been `expiring_soon` with an unchanged expiry for longer than `APP_CERT_RENEWAL_STALLED_THRESHOLD` (default 6h, Go duration syntax). The second case needs a `cert_expiry` alert rule, since the rule's evaluations record how long the expiry has been stuck.
:::

::: details I can't log in, or my session keeps dropping
If sign-in fails with "sign-in over plain HTTP is disabled", an `https://` dashboard URL is configured: open that URL instead. If it no longer works, set `APP_ALLOW_INSECURE_LOGIN=true` on the control plane (for install.sh installs, add `Environment=APP_ALLOW_INSECURE_LOGIN=true` to the systemd unit), restart it, sign in over HTTP, and fix or clear the dashboard URL on the Domains page.

On a fresh install the login page asks for a **setup token**. Print it with `sudo levelrail setup-token` on the server. Full detail: [Identity and access](identity-and-access.md#principals-a-session-or-a-token).
:::

::: details A node shows offline or won't enroll
The node agent dials **out** to the control plane, so check the *agent's* outbound connectivity first, not inbound firewall rules on the control plane. Confirm the join token hasn't expired and that the agent's clock isn't skewed (certificate validation is time-sensitive). See [Multi-node](multi-node.md#enrolling-a-second-node).
:::

::: details A managed database won't accept connections
Check whether public access is actually enabled for that database. It's off by default; enabling it needs an explicit port and bind address. See [Managing databases](managing-databases.md#public-access).
:::

::: details "docker: permission denied" when the control plane starts
The control plane needs access to the Docker socket. Add the user running it to the `docker` group, or run it as root if that's your deployment model. See [Docker](docker.md) for the exact socket path and permission model.

Running via the committed `docker-compose.yml`, the same error (visible in `docker compose logs`, e.g. `permission denied while trying to connect to the Docker daemon socket`) means `DOCKER_GID` doesn't match your host's actual docker group. Fix it:

```bash
export DOCKER_GID=$(getent group docker | cut -d: -f3)
docker compose up -d
```

The compose file falls back to `999` (the common Debian/Ubuntu default) if `DOCKER_GID` is unset, which is wrong on any host where the docker group has a different GID.
:::

::: details The data directory isn't writable
`levelrail-cli doctor`'s `data_dir_writable` check fails when the user running the control plane can't create a file in `APP_DATA_DIR`. This is almost always ownership or permissions drift, most commonly after restoring a volume from a backup as a different user, or a manual `chown` on the host. Fix it with `chown -R <service user> <data dir>` for a systemd install, or check the volume's ownership matches the container's `nonroot` user (uid/gid 65532) for the Docker image; see [Docker](docker.md) for that image's exact user model.
:::

::: details Port 80 or 443 is already in use
`levelrail-cli doctor`'s `port_80`/`port_443` checks fail when something other than this control plane's own embedded ingress already has the port bound. Find the culprit with `sudo ss -ltnp | grep -E ':80|:443'` (or `sudo lsof -i :80`). Common holders: an existing nginx, Apache, or standalone Caddy installation; a previous non-Docker install of this same platform still running; or a leftover process from a crashed prior instance. Stop or reconfigure that process, or move it off port 80/443, then re-run the check. This is a different problem from the port being blocked from the *outside*; see [Domains and ingress: firewall](domains-and-ingress.md#firewall-ports-80-and-443) for that case.
:::

::: details Ports 80/443 are open on the server but blocked by a firewall
`install.sh`'s own reachability self-test, and doctor's `external_reachability_80`/`external_reachability_443` checks, both warn rather than fail here, since a host firewall looks the same from outside as a closed port. Open both ports:

```bash
# ufw (Ubuntu/Debian)
sudo ufw allow 80/tcp && sudo ufw allow 443/tcp

# firewalld (RHEL/Fedora/Rocky)
sudo firewall-cmd --permanent --add-service=http --add-service=https && sudo firewall-cmd --reload
```

Then also check your cloud provider's firewall or security group rules; a host firewall being open doesn't mean the provider's edge is. `install.sh` can configure `ufw` for you on install with `LEVELRAIL_CONFIGURE_UFW=1`.
:::

::: details My domain won't resolve, or the setup wizard's DNS check stays red
Create an A (or AAAA for IPv6) record pointing the domain at your server's public IP, then check it actually propagated: `dig +short yourdomain.com` from your own machine, or [dnschecker.org](https://dnschecker.org/) to see it from multiple regions at once. A record you just created can take a few minutes to show up everywhere. If it never resolves, double check you edited the zone your domain's registrar actually uses, not a leftover one. See [Domains and ingress: setting up DNS](domains-and-ingress.md#setting-up-dns). No domain yet? Use the zero-config `sslip.io` URL the dashboard already shows instead.
:::

::: details A rollback target is missing
Levelrail pins the previous N images specifically so garbage collection can't orphan a rollback target. If one is still missing, check `levelrail-cli apps deploys list <name>` for what's actually retained, then see [Deploying apps](deploying-apps.md#rollback).
:::

## Clock skew

`levelrail-cli doctor`'s `clock_skew` check compares this host's clock against a remote HTTP `Date` header. A skew past the warning threshold (`APP_DOCTOR_CLOCK_SKEW_WARN`, default 5 minutes) usually means no NTP client is running. Install and enable one: `sudo systemctl enable --now systemd-timesyncd`, or `chronyd` if your distribution ships that instead. A wrong clock is a common, silent cause of certificate validation failures, since TLS checks a certificate's validity window against the local clock.

## External reachability could not be verified

The `external_reachability_80`/`external_reachability_443` checks dial this host's own public IP from itself. Many routers and cloud NAT setups don't support "hairpin" loopback (a LAN host reaching its own public address), so a failed dial here does **not** mean the port is actually unreachable from the internet, only that this particular self-test couldn't confirm it. Verify from an actual external vantage point instead: [canyouseeme.org](https://canyouseeme.org/), or `curl` from a different network. If it's genuinely closed, check your router's or cloud provider's port forwarding/security group rules for ports 80 and 443.

## Below the recommended minimum RAM or CPU

The `ram`/`cpu` checks warn when this host is below the recommended minimums (`APP_DOCTOR_MIN_RAM_BYTES`/`APP_DOCTOR_MIN_CPU_COUNT`, defaults 1GiB and 2 cores). This is a heads-up, not a hard requirement: a single small app can run fine below it. If you're seeing real slowness or OOM kills, add RAM/CPU or reduce the number of apps and concurrent builds on this box.

## Control plane backup is stale

The `control_plane_backup` check warns when the newest control plane snapshot is more than 3 days old. Scheduled snapshots may be failing (check the server log for `scheduled control plane backup failed`, often a full disk) or the server restarts more often than `APP_CONTROL_PLANE_BACKUP_INTERVAL` (default 24h). Take one now with `levelrail-cli control-plane-backups create`. See [Control plane backup and restore](/control-plane-backup).

## Control plane disaster recovery warning

The `control_plane_dr` check warns when encrypted off-box backups are off or unhealthy. The message names the problem: no recipient set, last run failed (the error is shown; a rejected credential or a full bucket are the usual causes), run overdue, no escrow bundle, no drill yet, a failed or overdue drill, or the escrow destination being the backup bucket. `levelrail-cli control-plane-backups schedule show` lists every warning. See [Disaster recovery](/disaster-recovery).

## "Can't reach the control plane" banner

The dashboard shows a red banner at the top when the API stops answering: network errors, or 502/503/504 responses from a reverse proxy, on two or more requests within 10 seconds. While it is showing, the dashboard pauses its 30 second polling so it does not pile up failing requests.

It probes the unauthenticated `GET /healthz` on its own with exponential backoff (2s, 4s, 8s, up to 30s). Press **Retry now** to probe immediately. When the probe succeeds, the banner disappears, a "Reconnected" toast appears and every query refetches. The banner can be dismissed with the X and returns on the next outage.

Common causes: the control plane restarted (check `systemctl status` or `docker logs`), your reverse proxy lost its upstream, or your own network dropped. If `/healthz` answers from the server itself but not through your proxy, the proxy is the problem.

An expired session is different: a 401 sends you to the login page, and after signing in you land back on the page you were on.

## Deploy preflight and failure diagnosis

Two read-only tools catch the common failures before and after a deploy. Both are deterministic rules over signals the platform already collects, with no model call, and neither changes anything by itself.

### Preflight

Run preflight from the app overview ("Run preflight"), the create-app dialog ("Check before creating"), `levelrail-cli apps preflight <name> [--require-env A,B]`, or the `preflight_app` MCP tool. Each check reports pass, warn or fail with a reason and a fix. The CLI exits 1 when any check fails.

| Check | Fails when | Warns when |
| --- | --- | --- |
| DNS for each domain | it resolves to an IP that is not this server's public IP | it does not resolve yet, it is proxied by Cloudflare, or the server IP could not be detected |
| Host port | a pinned host port is taken by another app or a process on the node | the check could not run |
| Image | the registry has no such image or tag | the registry needs credentials, is rate limiting, or is unreachable |
| Disk space | free space is below the image size times `APP_PREFLIGHT_DISK_FACTOR` (default 3) | free space is below `APP_PREFLIGHT_MIN_FREE_DISK_BYTES` (default 2 GiB) |
| Memory limit | the limit exceeds the node's total memory | the limit is below `APP_PREFLIGHT_MIN_MEMORY_BYTES` (default 128 MiB) or above the memory currently free |
| Git source | the branch does not exist or the host is unreachable | the repository is private (checked anonymously) or uses an SSH URL |
| Required env | a required variable is not set (including variables declared with an empty value) | never |
| Volumes and bind mounts | a bind mount path is relative or under a protected system path, or a volume path is not absolute | never |
| GPU | the node has no free GPU for the request | never |

Each check is bounded by `APP_PREFLIGHT_TIMEOUT` (default 8s). Image lookups are cached for `APP_PREFLIGHT_IMAGE_CACHE_TTL` (default 5m). Disk and memory are only known for the control plane's own node. A Cloudflare proxied domain is a warning, not a failure: it works with SSL mode Full (strict), or switch the record to DNS only until the certificate is issued.

### Failure diagnosis and one-click fixes

`GET /api/v1/apps/{name}/diagnose`, `levelrail-cli apps diagnose <name>`, the deploy failure card and the `diagnose_app_failure` MCP tool return typed causes, each with evidence lines and numbered fixes. A fix is one of:

- **patch**: exact field changes, previewed as a diff, then applied through the normal app update with your own permissions (and recorded in the audit log). Optionally followed by a redeploy.
- **input**: the same, but you supply a value (for example an env var).
- **manual**: no API setter exists (volume ownership, image architecture); the hint says what to do.

Apply from the CLI with `levelrail-cli apps diagnose <name> --apply-fix N [--input env.NAME=value] [--redeploy]`. A fix refuses to apply if the app changed since the diagnosis. The attention list marks apps that have a one-click fix.

| Cause | Evidence | Fix |
| --- | --- | --- |
| `WRONG_PORT` | listening sockets from `/proc/net/tcp` (needs exec access enabled and a running container), else a "listening on" log line or the image's exposed ports, together with a failing readiness check | patch `port` to the port the app actually listens on |
| `OOM_KILLED` | exit code 137 with the OOMKilled flag, or OOM lines in the logs | patch `resources.memory_bytes` up by `APP_DIAGNOSE_OOM_FACTOR` (default 2, at least 256 MiB more) |
| `MISSING_ENV` | "is not set", "is undefined", "required environment variable", Python `KeyError`, pydantic "Field required", zod "Required", Go envconfig | input `env.NAME` for each missing variable |
| `PORT_IN_USE` | "port is already allocated", `EADDRINUSE`, "address already in use" | manual: free the port or pin another host port |
| `PERMISSION_DENIED` | `EACCES`, `PermissionError`, "cannot create directory ... Permission denied" | manual: chown the host directory or volume to the container user |
| `EXEC_FORMAT_ERROR` | "exec format error", "no matching manifest for", platform mismatch | manual: rebuild for the node architecture or publish a multi-arch image |
| `COMMAND_NOT_FOUND` | "executable file not found in $PATH", "not found" from sh, exit code 127, entrypoint "no such file" | manual: fix the entrypoint, shebang, line endings or executable bit |
| `HEALTHCHECK_FAILING` | readiness probe timeout, refused connection, or a 404 | patch `health.readiness.path` to a candidate (`/healthz`, `/health`, `/`, ...) on a 404, otherwise a manual hint |
| `IMAGE_PULL_FAILED` | manifest unknown (not found), unauthorized (credentials), toomanyrequests (rate limit) | manual: fix the reference or add a registry credential |
| `BUILD_OUT_OF_DISK` | "no space left on device", `ENOSPC` | manual: prune images and build cache or add disk |
| `CRASHLOOP_GENERIC` | crashloop alert firing or a non-zero exit, with no more specific cause | manual: read the logs right after startup |

Log text is untrusted: every excerpt is cleaned and secret-redacted before it leaves the server, and the MCP tools mark their output as untrusted.

## Readiness (`/readyz`)

`GET /healthz` is a bare liveness check. `GET /readyz` is unauthenticated and cheap, and returns `200 {"ready":true,"checks":[...]}` only when the control plane can serve: the database answers a query, every migration is applied, and the reconcile engine has started. Otherwise it returns `503` and the failing check has `"status":"failing"`. A Docker daemon outage shows as `"status":"degraded"` on the `docker` check but keeps `ready` true, so the dashboard stays reachable to show it. `levelrail healthcheck --ready` runs the same probe from inside the container (the shipped image uses it for its `HEALTHCHECK`); it exits non-zero on 503. During startup the engine check fails for a moment, which is what the image's `--start-period` absorbs.

## Still stuck?

Open a [GitHub Discussion](https://github.com/glincker/levelrail/discussions) with your `app.yaml`, the relevant log output, and what you already tried. For anything that looks like a real bug, [file an issue](https://github.com/glincker/levelrail/issues) instead.

## See also

- [Getting started](getting-started.md) for initial setup and first deploy
- [Deploying apps](deploying-apps.md) for health checks and deploy strategies
- [Managing databases](managing-databases.md) for database-specific issues
- [CLI reference](cli-reference.md) for the `doctor` command and other diagnostics
- [Observability](observability.md) for accessing logs and metrics when debugging
