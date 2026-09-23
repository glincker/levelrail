---
description: Fixes for the most common problems when deploying, logging in, or running Levelrail.
---

# Troubleshooting

Start here for a fast fix. Each entry links to the full page if you need more depth.

::: details My deploy is stuck or failed
1. Check the build log first: dashboard's deploy detail page, or `levelrail-cli apps deploys logs <name> <deploy-id>`.
2. If the build succeeded but the app never came up, the readiness probe is the usual cause. Confirm the path in `app.yaml`'s `health.readiness` actually returns 2xx from inside the container, not just from your browser. A slow cold start (JVM warm-up, a large migration) can also legitimately take longer than the default 60s readiness budget: raise it with `health.readyTimeout` instead of treating the false `ReadinessFailed` as a real bug.
3. A crashlooping container gets its last 200 log lines surfaced automatically in the dashboard, no separate log search needed.
4. If this keeps happening on every deploy of a given app, consider turning on "Auto-rollback on crashloop" (Deploys tab, off by default) so the next crashloop redeploys the previous known-good image automatically instead of retrying the same bad one. See [Observability](observability.md).
5. Still stuck: [Deploying apps](deploying-apps.md#health-checks) covers the full health check contract.
:::

::: details TLS certificate won't issue
This has its own dedicated runbook: [ACME verification runbook](acme-verification-runbook.md). Start there; it covers DNS propagation, rate limits, and staging-vs-production ACME directories.
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
:::

::: details The data directory isn't writable
`levelrail-cli doctor`'s `data_dir_writable` check fails when the user running the control plane can't create a file in `APP_DATA_DIR`. This is almost always ownership or permissions drift, most commonly after restoring a volume from a backup as a different user, or a manual `chown` on the host. Fix it with `chown -R <service user> <data dir>` for a systemd install, or check the volume's ownership matches the container's `nonroot` user (uid/gid 65532) for the Docker image; see [Docker](docker.md) for that image's exact user model.
:::

::: details Port 80 or 443 is already in use
`levelrail-cli doctor`'s `port_80`/`port_443` checks fail when something other than this control plane's own embedded ingress already has the port bound. Find the culprit with `sudo ss -ltnp | grep -E ':80|:443'` (or `sudo lsof -i :80`). Common holders: an existing nginx, Apache, or standalone Caddy installation; a previous non-Docker install of this same platform still running; or a leftover process from a crashed prior instance. Stop or reconfigure that process, or move it off port 80/443, then re-run the check. This is a different problem from the port being blocked from the *outside*; see [Domains and ingress: firewall](domains-and-ingress.md#firewall-ports-80-and-443) for that case.
:::

::: details A rollback target is missing
Levelrail pins the previous N images specifically so garbage collection can't orphan a rollback target. If one is still missing, check `levelrail-cli apps deploys list <name>` for what's actually retained, then see [Deploying apps](deploying-apps.md#rollback).
:::

## Still stuck?

Open a [GitHub Discussion](https://github.com/glincker/levelrail/discussions) with your `app.yaml`, the relevant log output, and what you already tried. For anything that looks like a real bug, [file an issue](https://github.com/glincker/levelrail/issues) instead.

## See also

- [Getting started](getting-started.md) for initial setup and first deploy
- [Deploying apps](deploying-apps.md) for health checks and deploy strategies
- [Managing databases](managing-databases.md) for database-specific issues
- [CLI reference](cli-reference.md) for the `doctor` command and other diagnostics
- [Observability](observability.md) for accessing logs and metrics when debugging
