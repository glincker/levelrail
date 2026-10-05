---
description: Fixes for the most common problems when deploying, logging in, or running Levelrail.
---

# Troubleshooting

Find the symptom, apply the fix, and follow the link if you need more depth. Entries are grouped: problems with an app or deploy first, then problems with the server itself, then what each `doctor` finding means.

## Where to look first

Four commands cover most investigations. All of them work from your laptop once you have signed in with `levelrail-cli auth login`.

```bash
levelrail-cli attention              # everything failing right now: apps, nodes, certificates, doctor checks
levelrail-cli apps status NAME       # the app's reconcile conditions, each with a reason string
levelrail-cli apps logs NAME --follow
levelrail-cli doctor                 # preflight checks for Docker, disk, ports, and the database
```

Every reconcile pass writes a status condition with a reason, so `apps status` usually names the problem directly. The dashboard shows the same data on the app's Overview page and on the Status page (`/status`, with a badge in the sidebar). `attention` and the Status page list failing apps, offline nodes, expired or expiring certificates, and doctor warnings or failures, critical first. The CLI exits 1 when any item is critical. A node listed as offline has a connection history: `levelrail-cli nodes events <id>`.

## Apps and deploys

<AccordionGroup>

<Accordion title="My deploy is stuck or failed" id="my-deploy-is-stuck-or-failed">

<Steps>
<Step title="Check the classified cause">

Run `levelrail-cli apps deploys show <name>`: a failed or blocked deploy carries a classified cause and fix, see [Deploy failures](deploy-failures.md).

</Step>
<Step title="Check the build log">

Dashboard's deploy detail page, or `levelrail-cli apps deploys logs <name> <deploy-id>`.

</Step>
<Step title="Build succeeded, app never came up">

The readiness probe is the usual cause. The `Ready` condition's message names the exact request or command and what came back (a status, a redirect target, a TLS error, an exec exit code). A 302 to a login page wants `follow_redirects` or `expected_status: 200-399`; a self-signed HTTPS endpoint wants `scheme: https` with `tls_skip_verify: true`; a database is better checked with an `exec` probe. See [Health checks](app-spec-reference.md#health-checks). A slow cold start (JVM warm-up, a large migration) can also legitimately take longer than the default 60s readiness budget: raise it with `health.readyTimeout` instead of treating the false `ReadinessFailed` as a real bug.

</Step>
<Step title="Check the crashloop logs">

A crashlooping container gets its last 200 log lines surfaced automatically in the dashboard, no separate log search needed.

</Step>
<Step title="Stop it from recurring">

If this keeps happening on every deploy of a given app, consider turning on "Auto-rollback on crashloop" (Deploys tab, off by default) so the next crashloop redeploys the previous known-good image automatically instead of retrying the same bad one. See [Observability](observability.md).

</Step>
<Step title="Still stuck?">

[Deploying apps](deploying-apps.md#deploy-and-watch-a-rollout) explains what a rollout does and how it fails, and `levelrail-cli apps diagnose <name>` runs the cause detection described under [Deploy preflight and failure diagnosis](#deploy-preflight-and-failure-diagnosis).

</Step>
</Steps>

</Accordion>


<Accordion title="An app shows CrashLoopBackOff" id="an-app-shows-crashloopbackoff">

The container keeps exiting. Levelrail restarts it immediately once, then waits 5s, 10s, 20s and so on up to 2 minutes between restarts instead of restarting on every exit, so the host and the control plane stay calm. Read the cause first: `levelrail-cli apps logs <name> --tail 50`, and `levelrail-cli apps status <name>` for the countdown to the next restart. Fix the image or its configuration and redeploy; a new deploy starts with a fresh delay. If you need the old restart-immediately behavior, set `APP_RESTART_BACKOFF_BASE=0`. A `crashloop` alert rule sends the last 200 log lines with its notification. See [observability](observability.md#alert-rules).

</Accordion>


<Accordion title="Creating a notification channel fails with &quot;points at an internal address&quot;" id="creating-a-notification-channel-fails-with-points-at-an-inte">

The webhook URL is a loopback, private or link-local address (or `localhost`), which the outbound SSRF guard would refuse on every send. Use a public URL, or if the receiver is on your own network set `APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true` in the control plane's environment, restart it, and create the channel again. See [security](security.md#outbound-requests-to-user-supplied-urls).

</Accordion>


<Accordion title="Invite or password reset emails contain a link with no host" id="invite-or-password-reset-emails-contain-a-link-with-no-host">

The link is built from the primary domain or, failing that, the dashboard URL. Set the dashboard URL (the Domains page, or `levelrail-cli settings dashboard-url set --url https://deploy.example.com`) and send the invite again.

</Accordion>


<Accordion title="I upgraded and want to go back" id="i-upgraded-and-want-to-go-back">

The control plane snapshots its database before applying new migrations. Start the older binary against the newer data and it refuses with `database schema is newer than this binary supports: database is at version N`; nothing is modified. Stop the service, then `levelrail restore-snapshot --list`, `--dry-run latest`, `latest`, and start the older binary. Anything written after the snapshot is lost, and `alerting.db` and `telemetry.db` are not rolled back by `restore-snapshot`. See [Installing](installing.md#rolling-back).

</Accordion>


<Accordion title="A rollback target is missing" id="a-rollback-target-is-missing">

Levelrail pins the previous N images specifically so garbage collection can't orphan a rollback target. If one is still missing, check `levelrail-cli apps deploys list <name>` for what's actually retained, then see [Deploying apps](deploying-apps.md#roll-back).

</Accordion>


<Accordion title="A managed database won't accept connections" id="a-managed-database-won-t-accept-connections">

Check whether public access is actually enabled for that database. It's off by default; enabling it needs an explicit port and bind address. See [Managing databases](managing-databases.md#public-access).

</Accordion>

</AccordionGroup>

## Login, domains, and ports

<AccordionGroup>

<Accordion title="I can't log in, or my session keeps dropping" id="i-can-t-log-in-or-my-session-keeps-dropping">

If sign-in fails with "sign-in over plain HTTP is disabled", an `https://` dashboard URL is configured: open that URL instead. If it no longer works, set `APP_ALLOW_INSECURE_LOGIN=true` on the control plane (for install.sh installs, add `Environment=APP_ALLOW_INSECURE_LOGIN=true` to the systemd unit), restart it, sign in over HTTP, and fix or clear the dashboard URL on the Domains page.

On a fresh install the login page asks for a **setup token**. Print it with `sudo APP_DATA_DIR=/var/lib/levelrail-data levelrail setup-token` on the server. Full detail: [Identity and access](identity-and-access.md#principals-a-session-or-a-token).

</Accordion>


<Accordion title="TLS certificate won't issue" id="tls-certificate-won-t-issue">

This has its own dedicated runbook: [ACME verification runbook](acme-verification-runbook.md). Start there; it covers DNS propagation, rate limits, and staging-vs-production ACME directories.

To see whether renewal is failing, run `levelrail-cli domains certificates`. The `RENEWAL` column is `ok` or `stalled`, and the same value is the `renewal` field of `GET /api/v1/certificates`. The dashboard shows a "Renewal stalled" badge on the domain row and in the domain editor. A certificate is `stalled` when it has already expired, or when it has been `expiring_soon` with an unchanged expiry for longer than `APP_CERT_RENEWAL_STALLED_THRESHOLD` (default 6h, Go duration syntax). The second case needs a `cert_expiry` alert rule, since the rule's evaluations record how long the expiry has been stuck.

</Accordion>


<Accordion title="My domain won't resolve, or the setup wizard's DNS check stays red" id="my-domain-won-t-resolve-or-the-setup-wizard-s-dns-check-stay">

Create an A (or AAAA for IPv6) record pointing the domain at your server's public IP, then check it actually propagated: `dig +short yourdomain.com` from your own machine, or [dnschecker.org](https://dnschecker.org/) to see it from multiple regions at once. A record you just created can take a few minutes to show up everywhere. If it never resolves, double check you edited the zone your domain's registrar actually uses, not a leftover one. See [Domains and ingress: setting up DNS](domains-and-ingress.md#setting-up-dns). No domain yet? Use the zero-config `sslip.io` URL the dashboard already shows instead.

</Accordion>


<Accordion title="Port 80 or 443 is already in use" id="port-80-or-443-is-already-in-use">

`levelrail-cli doctor`'s `port_80`/`port_443` checks fail when something other than this control plane's own embedded ingress already has the port bound. Find the culprit with `sudo ss -ltnp | grep -E ':80|:443'` (or `sudo lsof -i :80`). Common holders: an existing nginx, Apache, or standalone Caddy installation; a previous non-Docker install of this same platform still running; or a leftover process from a crashed prior instance. Stop or reconfigure that process, or move it off port 80/443, then re-run the check. This is a different problem from the port being blocked from the *outside*; see [Domains and ingress: firewall](domains-and-ingress.md#firewall-ports-80-and-443) for that case.

</Accordion>


<Accordion title="Ports 80/443 are open on the server but blocked by a firewall" id="ports-80-443-are-open-on-the-server-but-blocked-by-a-firewal">

`install.sh`'s own reachability self-test, and doctor's `external_reachability_80`/`external_reachability_443` checks, both warn rather than fail here, since a host firewall looks the same from outside as a closed port. Open both ports:

<Tabs :items="['Ubuntu/Debian (ufw)', 'RHEL/Fedora/Rocky (firewalld)']">
<Tab value="Ubuntu/Debian (ufw)">

```bash
sudo ufw allow 80/tcp && sudo ufw allow 443/tcp
```

`install.sh` can configure `ufw` for you on install with `LEVELRAIL_CONFIGURE_UFW=1`.

</Tab>
<Tab value="RHEL/Fedora/Rocky (firewalld)">

```bash
sudo firewall-cmd --permanent --add-service=http --add-service=https && sudo firewall-cmd --reload
```

</Tab>
</Tabs>

Then also check your cloud provider's firewall or security group rules; a host firewall being open doesn't mean the provider's edge is.

</Accordion>

</AccordionGroup>

## The server and its nodes

<AccordionGroup>

<Accordion title="&quot;docker: permission denied&quot; when the control plane starts" id="docker-permission-denied-when-the-control-plane-starts">

The control plane needs access to the Docker socket. Add the user running it to the `docker` group, or run it as root if that's your deployment model. See [Docker](docker.md) for the exact socket path and permission model.

Running via the committed `docker-compose.yml`, the same error (visible in `docker compose logs`, e.g. `permission denied while trying to connect to the Docker daemon socket`) means `DOCKER_GID` doesn't match your host's actual docker group. Fix it:

```bash
export DOCKER_GID=$(getent group docker | cut -d: -f3)
docker compose up -d
```

The compose file falls back to `999` (the common Debian/Ubuntu default) if `DOCKER_GID` is unset, which is wrong on any host where the docker group has a different GID.

</Accordion>


<Accordion title="The data directory isn't writable" id="the-data-directory-isn-t-writable">

`levelrail-cli doctor`'s `data_dir_writable` check fails when the user running the control plane can't create a file in `APP_DATA_DIR`. This is almost always ownership or permissions drift, most commonly after restoring a volume from a backup as a different user, or a manual `chown` on the host. Fix it with `chown -R <service user> <data dir>` for a systemd install, or check the volume's ownership matches the container's `nonroot` user (uid/gid 65532) for the Docker image; see [Docker](docker.md) for that image's exact user model.

</Accordion>


<Accordion title="A node shows offline or won't enroll" id="a-node-shows-offline-or-won-t-enroll">

The node agent dials **out** to the control plane, so check the *agent's* outbound connectivity first, not inbound firewall rules on the control plane. Confirm the join token hasn't expired and that the agent's clock isn't skewed (certificate validation is time-sensitive). See [Multi-node](multi-node.md#enrolling-a-second-node).

</Accordion>

</AccordionGroup>

## What each doctor finding means

`levelrail-cli doctor` and the Status page report these checks. Each section below names the check code, what trips it, and the fix.

### Clock skew

`levelrail-cli doctor`'s `clock_skew` check compares this host's clock against a remote HTTP `Date` header. A skew past the warning threshold (`APP_DOCTOR_CLOCK_SKEW_WARN`, default 5 minutes) usually means no NTP client is running. Install and enable one: `sudo systemctl enable --now systemd-timesyncd`, or `chronyd` if your distribution ships that instead. A wrong clock is a common, silent cause of certificate validation failures, since TLS checks a certificate's validity window against the local clock.

### External reachability could not be verified

The `external_reachability_80`/`external_reachability_443` checks dial this host's own public IP from itself. Many routers and cloud NAT setups don't support "hairpin" loopback (a LAN host reaching its own public address), so a failed dial here does **not** mean the port is actually unreachable from the internet, only that this particular self-test couldn't confirm it. Verify from an actual external vantage point instead: [canyouseeme.org](https://canyouseeme.org/), or `curl` from a different network. If it's genuinely closed, check your router's or cloud provider's port forwarding/security group rules for ports 80 and 443.

### Below the recommended minimum RAM or CPU

The `ram`/`cpu` checks warn when this host is below the recommended minimums (`APP_DOCTOR_MIN_RAM_BYTES`/`APP_DOCTOR_MIN_CPU_COUNT`, defaults 1GiB and 2 cores). This is a heads-up, not a hard requirement: a single small app can run fine below it. If you're seeing real slowness or OOM kills, add RAM/CPU or reduce the number of apps and concurrent builds on this box.

### Disk write latency is high

The `disk_io_latency` check writes and fsyncs a 1MiB file to the data directory and warns when that took longer than `APP_DOCTOR_DISK_IO_WARN_MS` (default 200ms). Free space (`disk_space`) says nothing about this: a disk can have plenty of room left and still write slowly enough to make every deploy, log write, and SQLite commit feel stuck. Check for a saturated disk with `iostat`/`iotop`, a network volume under load, or a nearly-full disk whose remaining space is fragmented.

### Agent advertise host is unreachable

The `agent_advertise_reachability` check confirms `APP_AGENT_ADVERTISE_HOST`, the address a remote agent dials to reach this control plane, is actually reachable. It fails when that address is still the loopback default (`127.0.0.1`) while one or more nodes are enrolled, since a remote agent can never dial its own machine's loopback address to reach a different host. It warns when the configured address doesn't accept a connection from this host itself. Set `APP_AGENT_ADVERTISE_HOST` to this control plane's real, reachable hostname or IP before enrolling a second node, restart, then re-enroll or re-issue certificates for any node that joined before the fix.

The `mesh_hub_endpoint` check applies when `APP_MESH_ENABLED=1`: agents send WireGuard handshakes to `APP_AGENT_ADVERTISE_HOST` on UDP 51820. It fails when that host is loopback while nodes are enrolled (agents log `no known endpoint for peer`), and otherwise reminds you to allow inbound UDP 51820 on the control plane's firewall and cloud security group, which cannot be probed from the host itself.

### Node stuck pending: "join token already used"

An agent that logs `join token already used` is retrying with a spent token. The node row stays `pending` and the dashboard and `levelrail-cli nodes list` flag it "Never connected" after five minutes. The token cannot be reused: delete the node, fix the cause the agent logged (an unwritable identity directory is the usual one; current agents check this before sending the token), and enrol again with a new join token. See [Multi-node](multi-node.md#step-3-confirm-it-registered).

### Registry reachability failed

The `registry_reachability_<host>` checks probe outbound HTTPS connectivity to every registry this control plane actually pulls or pushes against: each external registry credential's host, the built-in registry when enabled, and Docker Hub (`registry-1.docker.io`) when neither is configured. A warning here means builds and deploys against that specific registry will fail with a pull or push error until the connection is fixed (egress firewall rules, `HTTP(S)_PROXY` settings, or the registry itself being down).

### Control plane backup is stale

The `control_plane_backup` check warns when the newest control plane snapshot is more than 3 days old. Scheduled snapshots may be failing (check the server log for `scheduled control plane backup failed`, often a full disk) or the server restarts more often than `APP_CONTROL_PLANE_BACKUP_INTERVAL` (default 24h). Take one now with `levelrail-cli control-plane-backups create`. See [Control plane backup and restore](/control-plane-backup).

### Control plane disaster recovery warning

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
