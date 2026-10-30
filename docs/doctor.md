---
title: "Doctor"
icon: "/assets/icons/lifebuoy.svg"
sidebarTitle: "Doctor"
description: One health check for the server, in the dashboard and the CLI, with a fix or a next step for every problem.
"og:url": "https://docs.levelrail.com/doctor"
---

Doctor runs one set of checks on the control plane. The dashboard page (Settings, Doctor), `levelrail-cli doctor` and `GET /api/v1/system/doctor` all call the same server-side engine, so they never disagree.

Each check reports `ok`, `warn`, `fail` or `unknown` with a plain message and, when useful, the raw evidence behind it (a path, the process holding a port, a count). A failing check offers one of three things:

- **Auto fix**: the control plane does it, after you confirm. See [Fix-it engine](/fix-it).
- **Guided fix**: one thing you do elsewhere, then Re-check.
- **Manual**: an explanation and a next step.

Checks run in parallel, each with its own timeout (`APP_DOCTOR_CHECK_TIMEOUT`, default 10s). A slow check reports `unknown` and never holds up the others.

## In the dashboard

Open Settings, Doctor. Results appear as each check finishes, grouped by area, problems first. Every problem row has Fix now or a Re-check button. **Fix all safe issues** lists exactly what will change and applies only fixes that are one click and safe; anything that restarts a service or opens access stays a separate step. The dashboard home shows a banner linking here only when a check fails.

## In the CLI

```bash
levelrail-cli doctor              # same checks as the dashboard
levelrail-cli doctor --fix        # print a plan of safe auto fixes, ask, apply, re-run
levelrail-cli doctor --fix --yes  # apply without asking
levelrail-cli doctor --output json
levelrail-cli doctor disk         # only the disk checks, with the top consumers and a cleanup suggestion
levelrail-cli doctor paths        # where the CLI, control plane binary, releases dir, data dir and systemd unit live, and whether each exists (offline)
levelrail-cli doctor --local      # Docker, disk and ports only, no control plane needed
```

`doctor paths` never contacts a control plane. It takes `--json`, `--output` and `--query` like the other commands, prints the `APP_HTTP_ADDR` value when that variable is set, and uses `LEVELRAIL_RELEASES_DIR` and `APP_DATA_DIR` for the releases and data dirs when they are set. With no control plane configured (no token, no `--api-url`), `doctor` runs the local checks only. `--fix` always needs a control plane, applies only fixes that are automatic and safe, and lists the rest as skipped with the command to apply them on purpose (`fixes apply ID`). Without `--yes` it asks on a terminal and refuses to run otherwise. The exit code is 1 when any check fails.

Expected output:

```text
STATUS  CHECK                     MESSAGE
OK      Docker daemon             reachable, engine 27.3.1
WARN    Disk space                820 MiB free, below the 1.0 GiB warning threshold
OK      Port 443 available        in use by this control plane's own ingress, as expected
FAIL    Agent on worker-2         agent is not connected

Fixes:
  Disk space:
    automatic fix available (disk.reclaim), apply with: doctor --fix
  Agent on worker-2:
    evidence: status offline, last seen 3h0m0s ago, address 203.0.113.9
    on worker-2: check the agent service (systemctl status levelrail-agent), and that it can reach this control plane

One or more checks failed. See above.
```

## API

| Request | Purpose |
| --- | --- |
| `GET /api/v1/system/doctor` | Run everything, return one JSON report |
| `GET /api/v1/system/doctor/checks` | List the registered checks |
| `GET /api/v1/system/doctor/stream` | Server-sent events, one `check` event per finished check, then `fixes` and `done` |
| `POST /api/v1/system/doctor/checks/{id}/run` | Re-run one check |
| `POST /api/v1/fixes/{id}/apply` | Apply a fix (root, audit logged) |

## Checks

| Group | Check | What it looks at |
| --- | --- | --- |
| Container runtime | `docker` | Daemon reachable, engine version |
| | `install_state` | Whether an upgrade is available, you are up to date, or this is a development build whose database is newer than every release |
| | `container_runtime`, `docker_privilege`, `docker_guard`, `container_hardening` | Engine kind, privileges, API guard, hardening |
| | `network_pool` | Docker network address pool usage, with an auto fix for orphans |
| | `docker_orphans` | Unreferenced images, build cache, anonymous volumes, orphan networks (`APP_DOCTOR_ORPHAN_WARN_BYTES`) |
| This server | `disk_space` | Free space on the data volume, with an auto cleanup fix |
| | `inodes` | Free inodes on the data volume (`APP_DOCTOR_INODE_WARN_PERCENT`) |
| | `docker_root_disk` | Free space under Docker's storage root |
| | `disk_hygiene` | Free space on the data dir and Docker root, Docker image, build cache, volume and container sizes (largest first) and a suggested cleanup command; warns below `APP_DOCTOR_DISK_WARN_FREE_PCT` percent free (default 10). Read-only, nothing is deleted |
| | `data_dir_writable`, `ram`, `cpu`, `disk_io_latency`, `database` | Data directory, memory, CPU, disk latency, SQLite |
| Network | `port_80`, `port_443` | Who holds the ingress ports |
| | `firewall` | Host firewall rules for ports 80 and 443 |
| | `public_ip`, `external_reachability_*`, `acme_reachability`, `clock_skew` | Outbound and inbound reachability, ACME, clock |
| | `dns_domains` | Whether each configured domain resolves (summary only; use `domains doctor DOMAIN` for the full diagnosis) |
| | `certificates` | Expired or stalled certificates that need an operator |
| | `dashboard_url_https` | Dashboard URL still on plain http with certificates available, with an auto fix |
| | `registry_reachability_*`, `cross_node_ingress`, `ingress_edge`, `ingress_survives`, `exposure` | Registries, cross-node routing, edge policy, data plane resilience, exposed services |
| Security | `master_key_rotation`, `stale_secrets`, `notification_channel_secrets`, `secret_reveal_policy`, `legacy_tokens`, `git_connections`, `signature_pin` | Key and secret age, channel secrets, tokens, git provider connections, release signatures |
| Backups | `control_plane_backup`, `control_plane_dr` | Newest control plane snapshot and recovery readiness, with an auto snapshot fix |
| | `backup_freshness` | Scheduled database backups that are overdue |
| | `backup_drill` | The latest restore drill of every database and volume |
| Nodes and mesh | `node_agent_*` | One per remote node: agent connected, last seen |
| | `agent_version_skew` | Agents older than the minimum this control plane supports |
| | `wireguard`, `udp_port`, `public_address` | Mesh readiness, only when a remote node exists |
| Hardware | GPU and network share checks | GPU attach and placement, NAS client tools |

Fixes never delete user data or volumes. The cleanup fix removes only unreferenced Docker data and keeps rollback targets; anything destructive is a manual step.

See also [Troubleshooting](/troubleshooting) and the [Fix-it engine](/fix-it).
