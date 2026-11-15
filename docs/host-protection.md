---
title: "Host protection"
description: Keep the control plane alive on a crowded host, see when the kernel killed it for memory, and add swap safely. Beta.
"og:url": "https://docs.levelrail.com/host-protection"
---

Beta. On a small host with no swap, the kernel's out-of-memory killer can end the control plane even when it uses little memory itself, because other containers and builds are what filled the RAM. systemd restarts it within seconds, so nothing looks wrong. This page covers what the installer does about it, how an unclean restart is shown, and how to add swap.

## What the installer writes

`install.sh`, `install.sh upgrade` and `install.sh protect` write one drop-in, `/etc/systemd/system/levelrail.service.d/30-protection.conf`. The main unit and the other drop-ins (such as `10-install.conf`, `20-restart-policy.conf` and any you added) are never touched.

| Setting | Default | Why |
| --- | --- | --- |
| `OOMScoreAdjust` | `-900` | The kernel kills processes with a higher score first, so containers and builds go before the control plane. |
| `Restart=always`, `RestartSec` | `5` seconds | A killed process comes back on its own. |
| `StartLimitBurst` in `StartLimitIntervalSec` | `20` in `600` seconds | A real crash loop stops instead of spinning forever. A single kill never gets near it. |
| `TasksMax` | `infinity` | A busy host never refuses the control plane a thread. |
| `MemoryLow` | `256M` | Asks the kernel to reclaim from other cgroups before this one. Ignored without cgroup v2 memory control. |

Exit status 78 (database newer than the binary) is still never restarted. `Nice` and I/O priority are left alone on purpose, since lowering them can starve the dashboard under load.

Preview without root and without changing anything:

```sh
sh install.sh protect --dry-run
```

Apply to an existing install, then restart when it suits you (the OOM score applies at process start):

```sh
sudo sh install.sh protect
sudo systemctl restart levelrail
```

The command is idempotent. If the file was edited by hand (its first line no longer says it was written by the installer) it is kept as it is. On a host without systemd it prints a message and does nothing.

### Overriding

Drop-ins sort by file name, so put overrides in `40-local.conf` in the same directory, then `systemctl daemon-reload` and restart. The installer reads these variables when it writes the file: `LEVELRAIL_OOM_SCORE_ADJUST`, `LEVELRAIL_RESTART_SEC`, `LEVELRAIL_START_LIMIT_BURST`, `LEVELRAIL_START_LIMIT_INTERVAL_SEC` and `LEVELRAIL_MEMORY_LOW`. Set `LEVELRAIL_PROTECT=0` to skip the file entirely.

## Seeing an unclean restart

At boot the control plane writes a small marker file in its data directory with its process id, start time and the kernel's `oom_kill` counter from `/proc/vmstat`. A graceful stop marks it clean. If the next boot finds a marker that was not marked clean, it logs a warning, keeps a short history and shows an attention item: **Control plane restarted after an unclean stop**.

When the `oom_kill` counter rose since the marker was written, the item says the host likely killed Levelrail for memory, and recommends adding swap, lowering build concurrency, setting app memory limits and checking which containers use the most memory. The marker holds no secrets and no process list. Where `/proc/vmstat` does not exist the item still appears but makes no claim about memory.

The item stays for `APP_UNCLEAN_RESTART_WINDOW` (default `72h`) and can be silenced like any attention item.

## Doctor row

**Control plane protection** (`unit_protection`) shows the running OOM score adjustment, whether the drop-in exists, swap for context, and recent unclean restarts. It warns when the process is not shielded or was restarted recently. Swap itself is judged by the host memory checks, not this row. `APP_PROTECTED_OOM_SCORE_ADJ` (default `-500`) is the score at or below which the process counts as protected.

## Adding swap

Levelrail never creates swap itself. A swap file is the quickest fix on a host without one. This adds 2 GB, so change `2G` to what you can spare:

```sh
sudo sh -c 'fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile && echo "/swapfile none swap sw 0 0" >> /etc/fstab'
```

Caveats:

- `fallocate` files do not work as swap on some filesystems (btrfs, and ZFS in general). Use `dd if=/dev/zero of=/swapfile bs=1M count=2048` on ext4 or XFS if `swapon` complains, and follow your filesystem's guidance otherwise.
- `chmod 600` matters: a world-readable swap file can leak memory contents.
- Swap on a slow or network disk, or on a shared cloud disk with a small I/O quota, can make the host crawl instead of crash. Prefer more RAM when you can.
- Check the result with `swapon --show` and `free -h`. Some container hosts and VPS plans disallow swap.
- Swap buys time. Lower build concurrency and set app memory limits too, see [Build performance](/build-performance).
