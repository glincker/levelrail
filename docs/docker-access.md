---
description: Every Docker Engine API call Levelrail makes, why it needs it, and the guard that refuses everything else, including privileged containers and host mounts.
---

# Docker access and the API guard

The Docker socket is root equivalent: anything that can talk to it can start a privileged container that mounts `/` from the host. The control plane and every node agent hold that socket, so a bug or a compromise in either one is, by default, root on the machine.

Levelrail narrows this with an in-process **Docker API guard**. Each process starts a private Unix socket proxy in front of the real daemon, and every Docker client inside the process dials the proxy instead of the daemon. The proxy forwards only the endpoints listed below, and for container create, update, exec and volume create it reads the request body and refuses configurations that would hand a container the host.

## Modes

| `APP_DOCKER_GUARD` | Behavior |
| --- | --- |
| `audit` (default) | Everything is forwarded. A request a rule would deny is logged and written to the audit log as `docker_guard.would_deny`. |
| `enforce` | A denied request gets `403` with the rule id and never reaches Docker. Every denial is written to the audit log as `docker_guard.denied` and logged with the rule id. |
| `off` | No proxy. Clients dial the daemon directly, as before the guard existed. |

`audit` is the default so an upgrade never breaks a running deploy. After a week of audit mode with no would-be denials, the attention feed offers the switch to `enforce`. Change the mode from **Settings, Security, Docker API guard**, or with the CLI:

```
levelrail-cli docker-guard status
levelrail-cli docker-guard set --mode enforce
```

The API is `GET` and `PUT /api/v1/system/docker-guard` (`PUT` needs the `root` ability). Switching between `audit` and `enforce` applies immediately. Switching to or from `off` takes effect at the next restart; until then `off` behaves like `audit`. When `APP_DOCKER_GUARD` is set on the server it wins and the API returns `409`. An unrecognized value fails closed to `enforce`, and in `enforce` mode a guard that cannot start stops the process instead of running unguarded.

`levelrail-cli doctor` reports two checks: `docker_guard` (the mode and what it flagged in the summary window) and `docker_privilege` (whether the daemon is rootless or uses userns-remap, and whether the service user is root or in the `docker` group).

### Settings

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_DOCKER_GUARD` | `audit` | `off`, `audit` or `enforce`. Overrides the setting saved from the dashboard. |
| `APP_DOCKER_GUARD_SOCKET` | `<data dir>/guard/docker-guard.sock` | Where the proxy listens. Its directory must be `0700`; the socket is `0600`. |
| `APP_DOCKER_GUARD_ALLOW_HOST_NETWORK` | `false` | Lets a container declared with host networking through `network_host`. |
| `APP_DOCKER_GUARD_MAX_BODY_BYTES` | `4194304` | Largest create, update, exec or volume body the guard reads. Larger is denied. |
| `APP_DOCKER_GUARD_AUDIT_DEDUP` | `1h` | In audit mode, one audit row per rule and endpoint per interval. Enforce mode records every denial. |
| `APP_DOCKER_GUARD_SUMMARY_WINDOW` | `168h` | Window for the doctor, attention item and status summary. |
| `APP_DOCKER_GUARD_HEADER_TIMEOUT` | `30s` | Time a client has to send request headers to the proxy. |
| `APP_DOCKER_GUARD_RECORD_QUEUE` | `256` | Decisions waiting to be written. A full queue drops audit rows, never requests. |

The mode chosen in the dashboard is saved to `<data dir>/docker-guard.json`. A node agent reads `APP_DOCKER_GUARD` only and writes its decisions to `<data dir>/docker-guard-audit.jsonl` on that node.

## Rules

Every rule id appears in the `403` body, the log line and the audit row (`ability` column).

| Rule | Denies |
| --- | --- |
| `endpoint_not_allowed` | Any method and path not in the allowlist below: plugins, swarm, services, secrets, attach, commit, export, classic `/build`, prune of containers, volumes or networks. |
| `path_noncanonical` | Paths with `..` or `.` segments, encoded `/` or `\` (`%2F`, `%5C`), or NUL. Repeated slashes are collapsed and the version prefix (`/v1.47/`) is kept, so the daemon receives exactly the path that was matched. |
| `body_too_large`, `body_invalid` | A body over the limit, or one that does not decode as the endpoint's JSON type. |
| `privileged` | `HostConfig.Privileged`. |
| `host_pid`, `host_ipc`, `host_uts`, `host_userns`, `host_cgroupns` | `PidMode`, `IpcMode`, `UTSMode`, `UsernsMode`, `CgroupnsMode` set to `host`. `container:<id>` sharing stays allowed. |
| `network_host` | `NetworkMode: host`, unless the create was declared with host networking and `APP_DOCKER_GUARD_ALLOW_HOST_NETWORK=true`. `app.yaml` has no host networking field today, so in practice this is always denied. |
| `cap_add_disallowed` | A `CapAdd` entry outside the hardening profile: the minimal set from [Container hardening](/security#container-hardening) plus `APP_CONTAINER_HARDENING_CAP_ADD`. `NET_ADMIN` and `NET_RAW` pass only for a create that declared them (the egress sidecar). |
| `devices_not_granted` | `DeviceRequests`, or `Devices` other than `/dev/nvidia*` and `/dev/dri/*`, on a container whose app did not request a GPU. Also device changes through container update. |
| `device_cgroup_rules` | Any `DeviceCgroupRules`. |
| `security_opt_weakened` | `seccomp=` (any override, including `unconfined`), `apparmor=unconfined`, `label=disable`, `label=type:spc_t`, `systempaths=unconfined`, `no-new-privileges=false`. |
| `masked_paths_override` | Any `MaskedPaths` or `ReadonlyPaths`, which replace the default masking of `/proc` and `/sys`. |
| `bind_sensitive_path` | A bind mount (legacy `Binds` or `Mounts`) of a protected host path, or of a path that resolves to one through a symlink, or of an ancestor of one: `/`, `/etc`, `/root`, `/boot`, `/sys`, `/proc`, `/dev`, `/run`, `/var/run`, `/var/run/docker.sock`, `/var/lib/docker`, `/var/lib/containerd`, `/lib/modules` and the Levelrail data directory. The app volume model refuses these paths too, so no declaration can open them. |
| `bind_undeclared` | A bind mount whose host path the app volume model did not declare for this container. Named volumes and `tmpfs` are always allowed. |
| `mount_type_disallowed` | A mount type other than `volume`, `bind` or `tmpfs`. |
| `volumes_from` | `VolumesFrom`, which inherits another container's mounts. |
| `volume_bind_driver_opts` | A volume, created directly or inline in a mount, with the local driver's `o=bind` (or `rbind`) option, the trick that turns a named volume into a host bind mount. NFS and CIFS options pass. |
| `exec_privileged` | An exec with `Privileged: true`. |
| `image_import` | `POST /images/create?fromSrc=`, importing a root filesystem instead of pulling an image. |

### How declarations work

`internal/docker`'s `Create` builds every app, database and sidecar container from the stored app spec. Right before the create call it registers a declaration for that container name: its bind host paths, whether it asked for a GPU, host networking, and extra capabilities. The guard looks the declaration up by the `name` query parameter and drops it when `Create` returns. A create request that did not come through `Create`, or names a container `Create` is not creating at that moment, carries no grants. In enforce mode the guard forwards a re-encoded copy of the body it validated, so unknown fields and case or duplicate key tricks never reach the daemon in a form the guard did not see.

## The Engine API surface Levelrail uses

This is the allowlist. Paths are shown without the version prefix; `*` is one path segment and `**` is one or more (image references contain slashes).

| Method and path | Used by | Why |
| --- | --- | --- |
| `GET`, `HEAD /_ping`, `GET /version`, `GET /info` | client setup, doctor, GPU and CDI detection | API version negotiation, daemon health, runtime, storage root, security options |
| `GET /events` | reconciler | Observed state comes from the event stream, not polling |
| `GET /system/df` | status, orphaned volume sizes | Disk usage |
| `POST /auth` | registry credential test | `docker login` check |
| `GET /containers/json`, `GET /containers/*/json` | reconciler, orphan sweep, browser and one-shot helpers | List and inspect |
| `POST /containers/create` | app, database, sidecar, one-shot scanner, headless browser, volume chown helper | Body validated |
| `POST /containers/*/start`, `stop`, `kill`, `wait`, `DELETE /containers/*` | reconciler, helpers | Lifecycle |
| `POST /containers/*/update` | resource changes without recreate | Body validated |
| `GET /containers/*/logs`, `GET /containers/*/stats` | log store, live tail, metrics | Long-lived streams |
| `GET`, `HEAD`, `PUT /containers/*/archive` | volume ownership check, one-shot file injection | Copy files in or out of a container |
| `POST /containers/*/exec`, `POST /exec/*/start`, `POST /exec/*/resize`, `GET /exec/*/json` | terminal, database tools, backups | Exec create body validated; start is an HTTP upgrade (hijacked stream) |
| `GET /images/json`, `GET /images/**/json`, `DELETE /images/**`, `POST /images/**/tag`, `POST /images/prune` | rollback pinning, garbage collection | Image bookkeeping |
| `POST /images/create` | pull | Pull only, `fromSrc` denied |
| `POST /images/load`, `GET /images/get`, `GET /images/**/get` | built image load, image moves between nodes | Streamed tar archives |
| `GET /distribution/**/json` | digest resolution | Registry manifest lookup |
| `POST /grpc`, `POST /session`, `POST /build/prune` | BuildKit inside dockerd | HTTP upgrade to h2c for the BuildKit API, and build cache pruning |
| `GET /networks`, `GET /networks/*`, `POST /networks/create`, `POST /networks/*/connect`, `POST /networks/*/disconnect`, `DELETE /networks/*` | per-app networks, egress, bridge gateway | Networking |
| `GET /volumes`, `GET /volumes/*`, `POST /volumes/create`, `DELETE /volumes/*` | named volumes, NFS and CIFS shares | Volume create body validated |

Streaming and hijacked endpoints (`events`, `logs`, `stats`, exec start, `/grpc`, `/session`, image load and save) pass through unbuffered: responses flush as they arrive and upgraded connections are spliced end to end.

## What the guard does not protect against

- **A process that is already compromised at code execution level.** The guard runs inside the same process and the service user can still open the real socket. It stops bugs, injected specs, and a control plane asking an agent for something dangerous; it does not stop an attacker who can run arbitrary code as the service user. For that, run Docker rootless or with userns-remap, which `docker_privilege` reports.
- **The node agent container.** The default agent runs as root with the host Docker socket mounted. Its guard wraps its own Docker client, but anything running as root in that container can bypass it.
- **BuildKit build steps.** The guard sees the `/grpc` upgrade, not the BuildKit requests inside it. Builds run in BuildKit's own sandbox; insecure entitlements stay off unless the daemon itself allows them.
- **Image contents and allowed operations.** A pulled image runs with the hardened profile, but the guard does not judge what an image does. Exec into a running container, and copying files into it, stay allowed.
- **Symlinked bind sources on a containerized process.** When the process runs in a container it cannot see the host filesystem, so a bind source is checked by path only, not by what it resolves to on the host.
- **Agent denials in the dashboard.** An agent's denials are logged on the node and appended to its local audit file; the control plane sees them as the failed operation's error, which names the rule, not as audit rows.

## Prior art

| Candidate | Licence | What it gives | Decision |
| --- | --- | --- | --- |
| [Tecnativa docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) | Apache-2.0 | HAProxy in a container, one env var per API section plus a `POST` switch | Not used: section granularity cannot tell a hardened create from a privileged one, and it is a separate container to run |
| [wollomatic socket-proxy](https://github.com/wollomatic/socket-proxy) | MIT, parts Apache-2.0 | Go proxy with a per-method regex allowlist and bind mount source restrictions | Not used: same idea as ours, but a separate process configured by regex; we need per-container declarations from the app spec. No code copied |
| [Docker authorization plugins](https://docs.docker.com/engine/extend/plugins_authorization/) | Docker docs | Daemon-side allow or deny hook with the request body | Not used: needs daemon configuration and a restart on every host, and skips gRPC calls; a good later addition for operators who can configure the daemon |
| gVisor, Sysbox | Apache-2.0 | Stronger container sandboxes | Out of scope here: they confine containers, not the socket holder. Compatible with the guard |
| Portainer, Coolify, Dokploy | various | Mount the socket into their own container, or drive Docker over SSH, with full API access | Shows the gap: none restrict what their own process can ask Docker to do |
