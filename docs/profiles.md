---
title: "Server profiles"
icon: "/assets/icons/gear-six.svg"
sidebarTitle: "Server profiles"
description: Choose minimal, standard or suite to decide which optional parts of the control plane start, and switch later without losing data.
---

A profile is one setting that decides which optional subsystems start. The default, `standard`, is the install you have always had. `minimal` keeps a one-server install as light as Dokku. `suite` turns on everything for power users.

Idle CPU and memory were never the problem (see [Performance](/performance)). What a profile changes is how many things run, listen, warn and grow your disk, so there is less to explain on a small box.

## What each profile starts

| | `minimal` | `standard` (default) | `suite` |
| --- | --- | --- | --- |
| Mesh and WireGuard | off | off | on |
| Plugins and marketplace | off | off | on |
| Experimental features | off | off | all on |
| Metrics and log retention | 3 days | 15 days (7 days on a disk under 20 GiB) | same as standard |
| Backup scheduler and WAL shipping | start when a backup is scheduled | start at boot | start at boot |
| Agent listener (port 9443) | loopback only until a remote node exists | all interfaces | all interfaces |
| Security score | backup and key rotation are recommendations, not score items | scored | scored |
| Doctor RAM and CPU floors | 512 MiB and 1 core | 896 MiB and 2 cores | 896 MiB and 2 cores |
| Build SBOMs | off | off | on |
| Host disk, memory and patch history | with the mesh | with the mesh | on (mesh is on) |

A pipeline engine that only polls while a run is active, a retention default that follows disk size, and the corrected doctor floors apply to every profile.

## Choose a profile

At install time:

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh -s -- --profile minimal
```

`LEVELRAIL_PROFILE=minimal` does the same. The installer writes `Environment=APP_SERVER_PROFILE=minimal` to the systemd drop-in and keeps it on upgrades. A machine with under 2 GB of RAM is told that minimal fits it, but is never switched silently.

Later, from **Settings, Beta features, Server profile**, or from the CLI:

```bash
levelrail-cli settings profile get
levelrail-cli settings profile set suite --restart
levelrail-cli settings profile set minimal --dry-run
```

Only an administrator can change it. The choice is saved at once and applies at the next restart of the control plane, because most subsystems are wired at startup. `--restart` (or the **Restart now** button) asks for one.

## Which setting wins

1. `APP_SERVER_PROFILE` in the server environment. When set, the dashboard and CLI show it as pinned and refuse to change it.
2. The profile saved from Settings or the CLI.
3. `standard`.

Inside a profile the same order holds for each part: `APP_MESH_ENABLED` beats the mesh toggle in Network settings, which beats the profile. `APP_EXPERIMENTAL` beats a switch saved on the Beta features page, which beats the profile. A retention variable (`APP_METRICS_RETENTION`, `APP_LOGS_RETENTION`) beats any profile default.

## Moving between profiles never deletes data

Moving up only starts things. Moving down stops loops and makes gated routes return 404, and keeps every row. A few moves are checked first:

| Move | What happens |
| --- | --- |
| Down to `minimal` with remote nodes enrolled | Refused. Minimal binds the agent listener to loopback, so remote agents could not reach it. Remove the nodes or choose standard. |
| Away from `suite` while the profile is the only reason the mesh runs and nodes use it | Refused. Pin the mesh on in Network settings or remove the nodes. |
| Down while plugins are enabled | Refused. Disable them first so their containers are not left unsupervised. |
| Down to a shorter retention | Needs a confirm (`--confirm`), because the next hourly sweep deletes samples and log lines older than the new window. |

`--dry-run` prints the same checks without saving. The profile is stored in the control plane database, so restoring an older snapshot restores the older profile.

## Tuning knobs

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_SERVER_PROFILE` | unset | Pins the profile to `minimal`, `standard` or `suite`. |
| `APP_PROFILE_MINIMAL_RETENTION` | `72h` | Retention on the minimal profile. |
| `APP_SMALL_DISK_BYTES` | 20 GiB | Data disks below this get the shorter default retention. |
| `APP_SMALL_DISK_RETENTION` | `168h` | That shorter retention. |
| `APP_LOW_RAM_BYTES` | 2 GiB | Memory below which the dashboard recommends minimal. |
| `APP_PROFILE_LAZY_START_INTERVAL` | `5m` | How often minimal checks whether a deferred loop has work. |
| `APP_PIPELINE_IDLE_INTERVAL` | `1m` | Pipeline engine poll while no run is active. |

`APP_PROFILE` is a different setting: it names the CLI credentials profile.

## What it costs

Measured with the real boot harness on one machine (the table is in [Performance](/performance#server-profiles)): `minimal` runs 116 goroutines and 35 loops, `standard` 118 and 38, `suite` 128 and 41. The difference is small by design: the process was already light at about 162 MB. The gain is fewer things running, listening, warning and filling the disk, not a smaller process.
