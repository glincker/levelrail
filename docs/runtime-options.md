# Runtime options

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
| `cap_drop` | Capability names such as `NET_RAW`, or `ALL` |
| `pids_limit` | Process cap |

Counts and sizes are bounded; operators tune the bounds with the `APP_RUNTIME_*` variables.

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

Compose files map `tmpfs`, `ulimits`, `init`, `stop_grace_period`, `extra_hosts`, `dns`, `sysctls`, `cap_drop`, `pids_limit` and `read_only` the same way. A set that fails validation is skipped with a warning. Nodes with an older agent refuse the deploy until upgraded.
