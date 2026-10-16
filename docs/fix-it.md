---
title: "Fix-it engine"
icon: "/assets/icons/lifebuoy.svg"
sidebarTitle: "Fix-it engine"
description: Turn a failing readiness or doctor check into a previewable, verifiable fix.
---

A failing check used to be a sentence you had to translate into a command. The fix-it engine turns the common ones into typed fixes you can preview, apply, and re-verify from the dashboard or the CLI.

## What a fix is

Each fix belongs to one check and has a tier:

| Tier | What happens |
| --- | --- |
| `auto` | One click. The control plane makes the change, then re-runs the check. |
| `guided` | The exact command or snippet for your environment, plus a re-check button. |
| `manual` | A step only you can take, with the reason it cannot be automated here. |

Every fix also states its risk (`safe`, `restarts`, `exposes`), whether it can be undone, and a list of the concrete changes it will make before you confirm.

## Built-in fixes

| ID | Check | Tier | What it does |
| --- | --- | --- | --- |
| `mesh.public_address.set` | Control plane public address | auto | Saves the address agents dial as a setting that overrides `APP_AGENT_ADVERTISE_HOST`, no restart. Candidates come from interface addresses and the host you opened the dashboard with. |
| `mesh.wireguard.privileges` | WireGuard support | guided | Detects systemd, Docker, or a dev run, and prints only the snippet for that environment (a systemd drop-in with `AmbientCapabilities=CAP_NET_ADMIN`, docker flags, or run as root). |
| `mesh.udp_port.holder` | UDP port available | guided | Names the process or container holding the mesh UDP port and the command to stop it. The port is fixed, so there is no pick-another-port tier yet. |
| `disk.reclaim` | Disk space (the low-disk banner) | auto | Previews unused containers, dangling images, anonymous volumes, build cache, and orphaned networks with sizes, then prunes them. Rollback images are kept. |
| `docker.network_pool.prune` | App network pool | auto | Removes orphaned app networks to free subnets. |

## Loop protection

- A fix never applies without an explicit click. Re-checks may run on their own.
- Each attempt is stored with a fingerprint of the failing check result.
- After a failed attempt the fix drops one tier (auto, then guided, then manual) and shows "tried at T, still blocked because X".
- It re-arms only when the failing result changes.
- A mutating fix runs at most `APP_FIXIT_MAX_PER_HOUR` times per hour (default 3).

## CLI

```bash
levelrail-cli fixes list [--subject mesh] [--all]
levelrail-cli fixes preview mesh.public_address.set
levelrail-cli fixes apply mesh.public_address.set --param host=203.0.113.9 --yes
```

## API

`GET /api/v1/fixes`, `POST /api/v1/fixes/{id}/preview`, and `POST /api/v1/fixes/{id}/apply` (root). Apply responds with `fixed`, `still_failing`, `cannot_fix_here`, or `skipped`.

## Limits

The saved public address updates new join commands and readiness checks immediately. The agent TLS certificate and the mesh hub endpoint pick it up at the next control plane restart.

See also [Troubleshooting](/troubleshooting).
