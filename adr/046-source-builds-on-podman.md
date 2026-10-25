# ADR 046: Source builds on Podman through a managed buildkitd

Status: Proposed (v0.8.6 design, implementation in slices below)

Date: 2026-10-25

## Context

ADR 004 drives every source build (Dockerfile and Railpack) through the
BuildKit Go client. `build.NewClient` reaches BuildKit by hijacking the Docker
Engine API connection at `/grpc` (`dockerbuildkit.ClientOpts`), so it only
works where BuildKit runs inside `dockerd`. Podman serves a Docker-compatible
API but has no embedded BuildKit. A source deploy on Podman today dies on the
dial with a raw `unable to upgrade to h2c` gRPC error, which
`internal/failure` maps to `build_engine_unsupported` only by string pattern
(V3-1). `docs/container-runtimes.md` tells Podman users to build elsewhere.

Podman is the default engine on Fedora, RHEL, Rocky and Alma. Lifecycle parity
(stop before remove, network pool, nftables exposure) has landed; builds are
the remaining wall.

What the build path needs from an engine is narrow: a BuildKit gRPC endpoint to
solve against, and an image store to load the `ExporterDocker` tar into
(`ImageLoad`, which Podman's compat API serves). Build-cache admin
(`internal/docker/build_cache_admin.go`, used by `internal/janitor` and the
build center) calls the engine's build cache endpoints, which Podman does not
implement.

## Options

1. **Managed buildkitd container per build node.** The node runs the
   `moby/buildkit` rootless image, pinned by digest, as a Levelrail-managed
   container on the node's own Podman. The existing BuildKit client dials its
   unix socket directly instead of hijacking the engine connection. Solve
   code, Railpack LLB, cache import/export, progress streaming and remote
   dispatch (`SolveRemote`) stay unchanged.
2. **Buildah.** Use Buildah (CLI or its Go library) for Dockerfile builds on
   Podman hosts. Railpack emits BuildKit LLB, not a Dockerfile, so Railpack
   would still need BuildKit or a second frontend. Two build engines means two
   progress formats, two cache models (no registry cache export of the same
   shape) and a second set of failure classes. The Go library pulls
   containers/storage into the control plane binary and needs the same user
   namespace setup as option 1 anyway.
3. **Require Docker for builds.** Keep Podman for running apps; source builds
   need a Docker build node (`AcceptsBuildWorkloads`) or CI. Zero new code,
   but a single-node Podman host can never build from source, which is the
   first-hour experience this audience has.

## Decision

Option 1. Option 3 remains the documented fallback when option 1's preflight
fails, and Buildah is revisited only if rootless buildkitd proves unworkable on
the target distros (see Open questions).

Why: it keeps ADR 004 intact (one build engine, one client, one cache model,
Railpack included), the change is confined to how the client connects plus a
small supervisor, and the same buildkitd path later serves Docker nodes that
want builds isolated from `dockerd`.

### Connection

- `internal/build` gains a second constructor path: `NewClientAddr(ctx, addr,
  docker)` dials `unix://<socket>` with `bkclient.New` and keeps the Engine
  API client only for `ImageLoad`. `NewClient` keeps the hijack path for
  Docker.
- Selection is by runtime kind from `internal/docker` (`isPodman`, `RuntimeKind`),
  overridable with `APP_BUILDKIT_ADDR` (any `unix://` or `tcp://` BuildKit
  address, for operators who run their own) and `APP_BUILDKIT_MODE`
  (`auto`, the default; `engine` for the hijack; `managed`; `external` to use
  `APP_BUILDKIT_ADDR` only; `off` to disable source builds on the node).
- Before the first dial, a capability check asks the engine whether it is
  Podman. If it is and no managed or external buildkitd is usable, the build
  fails up front with a typed `ErrBuildEngineUnsupported` carrying the reason
  (no user namespaces, preflight failed, mode `off`), instead of dialing and
  surfacing the h2c error. The pattern match in `internal/failure` stays only
  as a backstop.

### Rootless and rootful

- **Rootless Podman:** buildkitd runs in the same user's Podman as the apps,
  using `moby/buildkit:<version>-rootless` with `--oci-worker-no-process-sandbox`
  unset (process sandbox kept). It needs `/etc/subuid` and `/etc/subgid`
  ranges for that user, `kernel.unprivileged_userns_clone` (or the distro
  equivalent) enabled, and the container started with
  `--security-opt seccomp=unconfined --security-opt apparmor=unconfined` only
  if the preflight proves the default profile blocks `unshare`. Fedora and
  Rocky use SELinux, so the label stays enforced and the run needs
  `--security-opt label=type:container_runtime_t` only where the preflight
  shows denials. Each relaxation is chosen by the preflight, logged, and
  shown in doctor; none is on by default.
- **Rootful Podman:** the same rootless image, run as a non-root user inside a
  rootful Podman with a user namespace (`--userns=auto`). We do not run a
  privileged rootful buildkitd: that would make the build socket equivalent to
  root on the host.

### Snapshotter: native overlay versus fuse-overlayfs

- Default to BuildKit's native overlayfs snapshotter, which works rootless on
  kernel 5.13 and later (Fedora 41 and Rocky 9.x kernels qualify; Rocky 9
  needs the preflight to confirm, since RHEL backports differ).
- Fall back to `fuse-overlayfs` when native overlay in a user namespace fails
  the preflight (older kernels, some XFS without `ftype=1`). That requires
  `/dev/fuse` passed into the container.
- Last resort is the `native` (copying) snapshotter, slow and disk hungry; the
  supervisor uses it only when `APP_BUILDKIT_SNAPSHOTTER=native` is set
  explicitly, never automatically.
- `APP_BUILDKIT_SNAPSHOTTER` (`auto`, `overlayfs`, `fuse-overlayfs`, `native`)
  overrides the choice.

### Socket location and permissions

The build socket is as powerful as the engine socket for anyone who can reach
it (it can run arbitrary build steps and read cached layers). Rules:

- The socket lives on the host in a directory owned by the Levelrail service
  user, mode `0700`, socket `0600`: rootful under `$APP_DATA_DIR/buildkit/run/`,
  rootless under `$XDG_RUNTIME_DIR/levelrail/buildkit/`. It is bind mounted
  into the buildkitd container only; buildkitd listens on that unix socket and
  never on TCP.
- buildkitd has no published ports and joins no app network (`--network=none`
  for the daemon container; build steps get their own network namespace from
  BuildKit, with egress as BuildKit provides by default).
- `internal/bindmount`'s deny list gains the socket directory and the cache
  volume path, plus `/run/user` (rootless runtime dirs), so no app, template or
  compose service can mount the build socket. `/var/run` is already denied.
- The agent's own remote build path keeps the socket local to the node: the
  control plane never dials a build node's buildkitd over the network; it
  dispatches through the existing agent transport and `SolveRemote`.
- Doctor checks the mode of the socket and its directory and fails the check
  if either is group or world accessible.

### Cache volume, pruning and the disk guard

- BuildKit state lives in a named volume `levelrail-buildkit-state` on the node
  (rootless: in the user's Podman storage). It holds the snapshotter content
  and the local cache.
- buildkitd's own GC is configured from env at container create:
  `APP_BUILDKIT_GC_KEEP_MB` (default 10240) and
  `APP_BUILDKIT_GC_KEEP_DURATION` (default `168h`), written into a generated
  `buildkitd.toml`.
- The build cache admin interface (`BuildCacheRecords`,
  `PruneBuildCacheOlderThan`) gets a buildkitd implementation using the
  client's `DiskUsage` and `Prune` calls, so the janitor and the build center
  work the same on Podman. The interface does not change.
- The disk guard: `checkDiskSpace` (`APP_MIN_BUILD_DISK_MB`) also checks the
  volume's backing path. When free space is below `APP_DISK_LOW_FREE_MB`, the
  supervisor runs a prune down to the GC keep size before the build starts;
  below `APP_DISK_CRITICAL_FREE_MB` the build is refused with the existing
  insufficient disk error instead of filling the disk.
- Registry cache (`APP_BUILD_CACHE_REGISTRY`) works unchanged since it is a
  solve option, not an engine feature.

### Registry push and pull, and short image names

- Base image pulls inside a build are done by buildkitd, not Podman. BuildKit
  resolves short names only against `docker.io`; it does not read
  `/etc/containers/registries.conf`. A Dockerfile `FROM nginx:1.27` therefore
  behaves the same as on Docker. We document that Podman's
  `unqualified-search-registries` does not apply to builds.
- Registry credentials for builds come from the same source the Docker path
  uses (session auth provider in the solve), not from Podman's
  `auth.json`.
- The finished image is exported as a docker tar and loaded with `ImageLoad`
  into the node's Podman store, same as today. It lands as
  `docker.io/library/<name>` or the fully qualified tag we set. The build path
  always tags with a fully qualified reference so Podman's short name
  resolution never runs on our own images (V3-3, F2-2 for builds).
- Pull of app images by short name stays on the existing path (404 digest
  lookup treated as unknown). Moving short name resolution to the Podman
  native API is a separate later slice and is not required for builds.

### Discovery and supervision

- The node agent (and the control plane's local node) owns the buildkitd
  container. It is a managed container with labels
  `levelrail.managed=true` and `levelrail.role=buildkitd`, created through the
  existing `internal/docker` client and the hardening defaults from
  `internal/docker/hardening.go`, minus what rootless buildkitd needs.
- Supervision is a small level-triggered loop in `internal/build/buildkitd`:
  desired state is "one buildkitd at the pinned digest, healthy", observed
  state is found by label. It creates, restarts with backoff, and replaces on
  digest mismatch. Health is a `ListWorkers` call over the socket with
  `APP_BUILDKIT_HEALTH_TIMEOUT` (default `5s`). Half-succeeded cases (container
  exists, socket missing; socket exists, container gone) each get a test, per
  the reconcile rules.
- It is started lazily on the first source build on that node, or at boot when
  `AcceptsBuildWorkloads` is set, and stopped after
  `APP_BUILDKIT_IDLE_STOP` (default `30m`, `0` keeps it running) so a node that
  rarely builds does not hold the memory.
- Node status reports `build_engine`: `docker-embedded`, `buildkitd-managed`,
  `buildkitd-external`, or `unsupported` with the reason. `SelectBuildNode`
  then skips nodes whose build engine is `unsupported` instead of routing a
  build there and failing.

### Upgrade path

- The image reference is a constant in `internal/build/buildkitd` (version tag
  plus `sha256` digest), bumped in code like any dependency and kept in step
  with the `moby/buildkit` client version in `go.mod` so client and daemon
  speak the same API. `APP_BUILDKIT_IMAGE` overrides it for air-gapped
  mirrors and must also be digest pinned (unpinned values are refused).
- On a Levelrail upgrade the supervisor sees the digest mismatch and replaces
  the container between builds only: it waits for in-flight solves (tracked
  by the existing build limiter) up to `APP_BUILDKIT_UPGRADE_WAIT` (default
  `10m`). The state volume is kept, so cache survives upgrades. If the new
  BuildKit cannot read the old state, the supervisor prunes the volume and
  logs it.

### Failure modes and errors

| Failure | Behavior |
| --- | --- |
| Engine is Podman, managed mode preflight fails (no subuid, userns off, overlay and fuse both fail) | Build fails before dial with `build_engine_unsupported` and the specific missing prerequisite plus the fix command. Node reports `unsupported`. |
| buildkitd image pull fails | Same class with the registry error; retried on next build. |
| Socket present but unhealthy | Supervisor restarts once inside the build's wait; then the build fails retryable with `buildkit_unavailable`. |
| buildkitd killed mid-build (OOM) | Solve error is surfaced; `build_oom` class when the container's exit is 137. |
| Disk below critical | Refused before solve with the insufficient disk error. |
| `ImageLoad` into Podman fails | Build fails with the load error; the tar is not retried silently. |

The `build_engine_unsupported` cause and fix text in
`internal/failure/classes.go` changes from "Podman has no BuildKit" to naming
the unmet prerequisite, and the doctor `build_engine` check prints the same
line.

## What we cannot verify on macOS

- Podman on macOS runs inside a Linux VM (`podman machine`) with its own kernel,
  SELinux mode and rootless setup, so it does not stand in for Fedora or Rocky.
  The managed buildkitd may start there, but that proves nothing about subuid,
  SELinux labels, overlay in a user namespace or kernel versions on a server.
- Unit tests on macOS can cover the selection logic, the supervisor against a
  fake engine, the preflight decision table, the prune adapter against a fake
  BuildKit client, and the bind mount deny list. Everything touching user
  namespaces, snapshotters, SELinux, socket permissions on a real host and
  `ImageLoad` into a real Podman store needs the droplets.

## Testing plan

- Unit: client selection by runtime and env; preflight decision table;
  supervisor create, replace on digest mismatch, restart, idle stop, and the
  half-succeeded cases; socket and directory mode check; deny list entries;
  prune adapter; typed unsupported error replacing the h2c pattern.
- Live (droplets, `scripts/qa/droplet.sh up --runtime podman --image <fedora
  or rocky>`): Fedora 41 rootless, Fedora 41 rootful, Rocky 9 rootless, Rocky 9
  rootful. On each: Dockerfile build, Railpack build (Node and Python), rebuild
  with warm cache, registry cache export and import, build dispatched from a
  control plane to a Podman build node, an app that tries to bind mount the
  socket directory (must be refused), disk guard prune with a small
  `APP_BUILDKIT_GC_KEEP_MB`, upgrade with a changed pinned digest, and the
  unsupported error on a host with subuid removed.
- `droplet.sh` currently installs with `apt-get`; it needs a `dnf` branch and
  a rootless setup step (service user, subuid, linger, user socket) before
  these runs are possible.

## Slices

| # | Slice | Files owned | Size |
| --- | --- | --- | --- |
| 1 | Typed unsupported error before dial, runtime based selection, `APP_BUILDKIT_MODE` and `APP_BUILDKIT_ADDR`, external address support | `internal/build/client.go`, `internal/failure/classes.go`, `cmd/levelrail/boot_loaders.go`, `cmd/levelrail-agent/main.go` | S |
| 2 | Supervisor and preflight: managed container, pinned image, socket dir, snapshotter choice, idle stop, upgrade | new `internal/build/buildkitd/` | O |
| 3 | Socket security: deny list, doctor socket mode check | `internal/bindmount/bindmount.go`, `internal/api/doctor*.go` | S |
| 4 | Cache admin and GC on buildkitd, disk guard prune | `internal/docker/build_cache_admin.go` (interface unchanged), new adapter in `internal/build/buildkitd/`, `internal/build/diskspace.go` | S |
| 5 | Node `build_engine` status and router skip | `internal/build/node.go`, `internal/build/router.go`, agent status payload (proto only if a field is needed) | S |
| 6 | `droplet.sh` dnf and rootless setup | `scripts/qa/droplet.sh`, `scripts/qa/droplet_test.sh` | H |
| 7 | Droplet QA run on the four targets, then docs: promote builds in `docs/container-runtimes.md`, short name note for builds | `docs/container-runtimes.md` | S |

Slice 1 ships alone and fixes V3-1 even before slice 2 exists. Slices 3 and 4
need slice 2's paths. Slice 7 gates moving builds on Podman from not supported
to best-effort.

### Settings (all from env)

`APP_BUILDKIT_MODE`, `APP_BUILDKIT_ADDR`, `APP_BUILDKIT_IMAGE`,
`APP_BUILDKIT_SNAPSHOTTER`, `APP_BUILDKIT_GC_KEEP_MB`,
`APP_BUILDKIT_GC_KEEP_DURATION`, `APP_BUILDKIT_HEALTH_TIMEOUT`,
`APP_BUILDKIT_IDLE_STOP`, `APP_BUILDKIT_UPGRADE_WAIT`, plus the existing
`APP_MIN_BUILD_DISK_MB`, `APP_DISK_LOW_FREE_MB`, `APP_DISK_CRITICAL_FREE_MB`
and `APP_BUILD_CACHE_*`.

## Consequences

- ADR 004 holds: one BuildKit client for every engine. ADR 004's note that
  there is "no separate buildkitd" is now true for Docker only.
- Podman nodes carry one more long-lived managed container, its memory while
  running, and a state volume that the janitor must manage.
- The build socket becomes a new privileged surface. It is contained by
  directory permissions, the bind mount deny list and the no-TCP rule, and
  doctor reports violations.
- The control plane binary does not grow a second build engine or the
  containers/storage dependency.
- `registries.conf` short name rules do not apply to builds; this is a
  documented difference, not a bug.
- Until slice 7 passes, builds on Podman stay labeled not verified in the docs.

## Open questions

- Does rootless buildkitd with native overlay work on stock Rocky 9 kernels,
  or does Rocky need fuse-overlayfs by default? Only the droplet run answers it.
- Which SELinux relaxations, if any, does buildkitd need under rootful Podman
  with `--userns=auto` on Fedora and Rocky? We want zero; the preflight must
  prove it.
- Should the managed buildkitd also become an option on Docker nodes (to
  isolate builds from `dockerd`), or stay Podman only for now?
- Idle stop default: is `30m` right, or should build nodes keep it running?
