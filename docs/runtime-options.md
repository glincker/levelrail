---
title: "Runtime options"
description: The allowlisted container options a service can set under runtime, with their bounds, and how compose keys map onto them.
"og:url": "https://docs.levelrail.com/runtime-options"
---

`runtime:` on a service sets an allowlisted subset of `docker run` options. A change recreates the container.

| Key | Meaning |
| --- | --- |
| `shm_size` | Size of `/dev/shm`, such as `256m` |
| `read_only` | Read-only root filesystem (same as `security.read_only_rootfs`) |
| `tmpfs` | `/path:size=64m[,mode=1777,uid=,gid=,exec,noexec,nosuid,nodev]`; size is required |
| `ulimits` | `nofile`, `nproc`, `cpu`, `core`, `stack`, `memlock`, `fsize`; an integer or `{soft, hard}` |
| `init` | Run a minimal init as PID 1 |
| `stop_timeout` | Seconds between SIGTERM and SIGKILL; the app may hold a stop (and so a redeploy) this long, up to the operator ceiling |
| `extra_hosts` | `hostname:ip` entries; `host-gateway` is refused |
| `dns` | Up to 3 resolver IPs, used after the mesh resolver |
| `sysctls` | `net.core.somaxconn`, `net.ipv4.ip_local_port_range`, `net.ipv4.ip_unprivileged_port_start`, `net.ipv4.ping_group_range`, `net.ipv4.tcp_fin_timeout`, `net.ipv4.tcp_keepalive_{time,intvl,probes}`, `net.ipv4.tcp_max_syn_backlog`, `net.ipv4.tcp_syncookies`, `net.ipv4.tcp_tw_reuse`; not with `egress` |
| `cap_drop` | Capability names such as `NET_RAW`, or `ALL`. The `CAP_` prefix is optional. |
| `pids_limit` | Process cap, at most `APP_RUNTIME_PIDS_LIMIT_MAX` (default 32768). Unset, the container keeps the platform default, `APP_CONTAINER_PIDS_LIMIT` (4096). |

Counts and sizes are bounded by the `APP_RUNTIME_*` variables, for example `APP_RUNTIME_DNS_MAX`, `APP_RUNTIME_ULIMIT_COUNT_MAX` and `APP_RUNTIME_TMPFS_MAX`.

Refused: `cap_add`, `privileged`, `devices`, `security_opt`, host `pid`, `ipc`, `uts`, `network_mode` and `userns_mode`, and host bind mounts. Each fails validation with the key named.

```yaml
version: 1
services:
  web:
    build: {type: image, image: nginx:1}
    port: 80
    runtime:
      init: true
      stop_timeout: 30
      tmpfs: ["/run:size=16m"]
      ulimits:
        nofile: 65536
```

Compose files map `tmpfs`, `ulimits`, `init`, `stop_grace_period` (to `stop_timeout`), `extra_hosts`, `dns`, `sysctls`, `cap_drop`, `pids_limit` and `read_only` onto the same keys. An invalid `cap_drop` or `read_only` fails the deploy. Any other invalid set is skipped with a warning that names it, and the rest of the block still applies. Nodes with an older agent refuse the deploy until upgraded.
