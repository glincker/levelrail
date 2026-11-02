# ADR 070: Desktop app and managed VM runtime

Status: Proposed

Date: 2026-11-01

Supersedes: ADR 057 (its "do not build the desktop app now" decision and its
process model). ADR 057's shell criteria table still holds and is reused
below.

## Context

The owner has decided to ship a desktop app. After Linux servers it is the
surface users ask for most. The goal: a user on macOS, Windows or Linux
installs Levelrail on their own computer and runs apps locally, including
desktop-style apps and widgets such as OpenClaw, inside a lightweight
isolated runtime with a local UI.

This is a different product from the one ADR 057 evaluated. ADR 057 asked
whether a laptop could host the control plane that manages remote servers
over SSH, and rejected it because public ingress lives in the control plane
(ADR 005) and stops when the laptop closes. This ADR is about hosting apps
on the laptop itself, for the person using the laptop. Nothing public is
promised, so the ingress objection in ADR 057 does not apply. Managing
remote servers from the desktop app stays out of scope (see Open
questions).

What exists on `release/v0.9.0`, read from the code:

1. **Local mode.** `internal/localmode` (`localmode.go`, `ports.go`,
   `state.go`, `docker.go`) runs a loopback-only control plane with
   `APP_LOCAL_MODE`, picks free loopback ports (`ChoosePorts`), refuses
   non-loopback listeners (`ValidateListenAddr`) and keeps state files at
   mode 0600. `cmd/levelrail-cli/dev.go` (plus `dev_unix.go`,
   `dev_windows.go`) supervises the `levelrail` binary and needs a Docker
   socket on the host. It is a no-isolation developer mode and stays that
   way.
2. **The control plane is Linux only in releases.** `.github/workflows/release.yml`
   builds `levelrail` with `GOOS: linux` (around line 90) and the CLI for
   `linux`, `darwin` and `windows` (matrix around line 119). Running the
   control plane inside a Linux VM on every desktop OS means no darwin or
   Windows control plane build is needed (ADR 057 fact 4, the Windows
   `Statfs` issue, stops mattering for this path).
3. **No VM runtime code exists.** A word-bounded grep for `lima`,
   `limactl` and `wsl` in `cmd`, `internal` and `.github` finds nothing.
   The only hit is `install-cli.sh:76`, which tells Windows users to use
   PowerShell or WSL. There is no `vm` subcommand in `cmd/levelrail-cli`.
4. **Runtime settings.** `internal/runtimesettings/service.go` resolves a
   setting from the store, then env, then the default, and records
   `Source`, `Apply` and `PendingRestart` per setting. The store lives in
   the control plane database, which in this design is inside the VM. VM
   memory, CPU and disk must be known before the VM exists, so they cannot
   live in that store (decision 2 handles this).
5. **Memory accounting.** `internal/footprint` reads host memory and sets a
   Go memory limit from `APP_GOMEMLIMIT_PERCENT`. Catalog templates carry
   `recommended_memory_bytes`.
6. **Platform mismatch handling.** `internal/failure/classes.go` defines
   `CodePlatformMismatch`, `internal/diagnose` maps exec format errors to a
   cause, and `internal/api/doctor_cloud.go` warns that an image built for
   another architecture fails with `PlatformMismatch` without emulation.
   An arm64 VM on Apple silicon hits this for amd64-only images.
7. **OpenClaw template.** `internal/catalog/contrib/openclaw.yaml` pins
   `ghcr.io/openclaw/openclaw:2026.10.5`, runs the gateway on 18789,
   publishes `18789:18789`, recommends 2 GiB and does not mount the Docker
   socket. OpenClaw is a headless gateway with a web Control UI. It needs no
   desktop environment. `docs/arm-compatibility.md` lists it as not tested
   on ARM.
8. **Streamed GUI apps.** The catalog has a Firefox template (VNC to the
   browser on 5800) and a headless browser service. No full desktop
   (Webtop, KasmVNC, Selkies, Xpra) template exists.
9. **Mac VM spike (2026-10-29, owner approved the direction).** A Lima VM
   with the vz driver, Ubuntu arm64, no mounts and a private Lima home
   booted in about 7 seconds on the owner's Mac, ran Docker and Levelrail
   from `install.sh`, and served the dashboard through a static loopback
   forward. Two findings shape this ADR: Lima 2.2 has no option that
   isolates the guest from the host network, so isolation must come from
   a firewall baseline inside the guest (the spike used a systemd unit
   with DOCKER-USER rules); and apps are reachable as
   `http://<app>.localhost` with `APP_PUBLIC_HOST=localhost` and the HTTP
   redirect off, the same settings `levelrail dev` uses. Nothing from the
   spike is in the repository.

## Decision (recommended)

Build it in three layers, CLI first, each shippable alone:

1. `levelrail vm` in the existing CLI, managing one Linux VM per user that
   runs the normal Linux control plane and Docker.
2. A small Go tray app that supervises that VM and opens the dashboard in
   the default browser.
3. The same Go app gains a system webview window around the dashboard.

The control plane, reconciler, catalog and dashboard are the same code
servers run. The desktop app is a window, a tray icon, a supervisor and an
updater. No product logic lives in it.

### 1. Shell technology: a thin Go shell

Decision: a Go shell (system webview plus a tray library, CGO on native
runners), reusing the Go VM code from the CLI as a library. Tauri and
Electron are rejected (see Alternatives).

| Criterion | Go shell | Tauri | Electron |
| --- | --- | --- | --- |
| Binary size | low megabytes plus the CLI code it links (not measured) | low megabytes (not measured) | adds a full browser engine, about 100 MB or more (not measured here) |
| Reuse of Go VM code | direct import, one codebase | sidecar process only, Rust glue | sidecar process only, Node glue |
| Signing and notarization | one Mach-O, one exe | one binary plus sidecar | runtime, helpers and sidecar |
| Auto update | our own signed manifest (decision 5) | built-in updater, but a second toolchain and key format | built-in updater needs platform signing, or a custom one |
| Team fit | Go and React only | adds Rust | adds a Node runtime |
| Rendering | three system webviews to test | three system webviews to test | one engine |

The Go shell wins on reuse and team fit, which matter most for a team that
ships one Go codebase. Tauri's real advantage is a mature updater and
packaging toolchain; this ADR accepts building a small updater instead
(decision 5). Which Go webview and tray libraries to use is picked in slice
S6 after a license and maintenance check; none is chosen here.

### 2. Runtime per OS

| OS | Runtime | Default |
| --- | --- | --- |
| macOS, Apple silicon | Lima, vz driver, Ubuntu LTS arm64 guest | supported first |
| macOS, Intel | Lima, vz driver, Ubuntu LTS x86_64 guest | best effort until tested on real Intel hardware (unverified that vz runs our template there) |
| Windows 10 and 11 | a dedicated WSL2 distro imported from our pinned root filesystem | after the macOS beta |
| Linux desktop | native Docker Engine (supported) or rootless Podman (best effort), no VM | with the macOS beta, labeled "no VM boundary" |

Lima details:

- A private Lima home under the app data directory, so the user's own Lima
  instances and ours never mix. Lima merges `_config/override.yaml` into the
  instance and it wins over our template (measured), so `lima.CheckHome` is
  mandatory.
- Lima is downloaded at `vm init` from its release, pinned by version and
  SHA-256 in our source, and stored in the data directory. No Homebrew
  requirement. A signed app may bundle it later (licensing, decision 6).
- The guest image is the upstream Ubuntu cloud image, pinned by SHA-256.
  We do not build or host a disk image in the first release. cloud-init
  installs Docker and the control plane with the existing `install.sh`,
  pinned to the CLI's own version.
- amd64 images on an arm64 VM: Rosetta for the guest stays off by default,
  so the existing `PlatformMismatch` path reports the problem. Turning it
  on is an open question.

Resource settings (owner rule: stored setting wins, env only seeds, applies
live where possible):

- VM settings live in a host-side settings file in the data directory,
  written by `vm init`. Env seeds the file on first init only. Later env
  changes are ignored, and `vm status` shows "env differs from stored
  value" so nothing changes silently. `levelrail vm set <key> <value>`
  changes the stored value.
- Each setting carries the same apply class as `runtimesettings`: live,
  on VM restart, or on recreate. Memory and CPU apply on VM restart; the
  CLI marks them pending and offers the restart. Disk can grow on a
  stopped VM and never shrinks (Lima grow support for our pin:
  unverified). Egress rules and port forwards apply live.
- 8 GB laptop budget: the default VM memory is 3 GiB, which leaves the host
  about 5 GB and fits the control plane plus OpenClaw (2 GiB recommended).
  Idle use at 2 GiB is measured (see Real boot results); the 3 GiB default is
  a planning figure. `vm init` refuses a memory
  value above a percent of host RAM and warns below a floor.
- On Windows the memory limit for WSL is global to all distros and needs a
  full WSL shutdown to change, so we never edit the user's WSL config.
  Instead the limit is a cgroup on the Docker daemon and control plane
  inside our distro (whether this caps the VM's real memory use as
  intended: unverified until the WSL spike).

### 3. Security boundary

Default posture: the VM sees nothing on the host that the user did not
choose to share.

- **Files.** No mounts by default, including no home directory mount (Lima
  mounts the home directory in its own default templates, ours removes
  that). Shared folders are explicit, one folder at a time, read-only by
  default, never the home root or a parent of the data directory, listed
  in `vm status`. On WSL, automount of Windows drives and Windows interop
  are turned off in the distro's `wsl.conf`, so the guest cannot reach host
  files or start host programs.
- **Docker socket.** The guest's Docker socket is never forwarded to the
  host, and app templates do not get it mounted (existing catalog policy).
  The host needs no Docker installation.
- **Ports.** Only two forwards exist: the dashboard and the app ingress,
  both bound to host loopback on ports chosen at init with
  `localmode.ChoosePorts` and persisted, so session cookies survive
  restarts. Lima's automatic port forwarding is disabled, so a port an app
  publishes inside the guest (OpenClaw publishes 18789) is not exposed on
  the host. Apps are reached through ingress at `<app>.localhost` and get
  the normal ingress shield and auth.
- **Network egress.** A guest firewall baseline, installed as a systemd
  unit before Docker starts, has output, input and forward chains. It denies
  guest and container traffic to the host (including through the gateway
  192.168.5.2) and to private, link-local and LAN ranges, allows DNS to the
  gateway on port 53 only, and leaves the internet open. The residual is
  that DNS uses the host resolver path. A template test asserts the rules
  exist; the real check runs on a real Mac on every Lima pin bump. A per-app
  "allow LAN" or "allow host" exception (for a local model server on the
  host) is an explicit, audited setting, off by default.
- **Control channel.** The CLI and the desktop app talk to the control
  plane over the loopback dashboard port with a per-install token. The
  token is created at `vm init`, stored in the OS keychain where available
  (else a 0600 file in the data directory), and exchanged once for an
  HttpOnly, SameSite=Strict session, as ADR 057 described. The control
  plane checks `Host` is the loopback address and port and checks `Origin`
  on state-changing requests. A host unix socket forward for CLI calls is
  a later hardening option, not a requirement, because the webview and
  browser need loopback HTTP anyway.
- **Clipboard and audio.** No host clipboard or audio bridge from the VM.
  The Lima template configures no sound device. A streamed desktop app
  gets clipboard and audio only through the user's browser session to
  that app, which the browser mediates.
- **GUI apps.** The supported default is apps with a web UI served through
  ingress (OpenClaw's Control UI, dashboards, notebooks). Streamed desktops
  (Webtop style images on KasmVNC or Selkies) are community templates,
  opt-in, with auth required, no host port, no Docker socket, not
  privileged and no `/dev/dri` by default, and marked "review before
  deploying". Native X11 or Wayland forwarding to the host is not
  supported. Reason: a streamed desktop is a full Linux session (often with
  passwordless sudo and heavy memory use) behind a web page, while a web UI
  app is one process behind our auth.

On Linux there is no VM. The boundary is the container runtime, and the app
says so. It does not change the host firewall of a user's desktop; the
egress baseline applies only inside our own Docker networks (how to do that
without touching host rules: open question).

### 4. Lifecycle and UX

CLI first. Every command works without the desktop app.

| Command | Does |
| --- | --- |
| `levelrail vm init` | checks OS, CPU virtualization and free memory and disk; fetches pinned Lima or imports the WSL distro; writes settings; creates the token; boots; waits for the control plane health check |
| `levelrail vm start` / `stop` | boots or gracefully stops the VM |
| `levelrail vm status` | state, versions, memory and disk use against limits, ports, shared folders, pending settings, isolation check result |
| `levelrail vm shell` | a shell in the guest, for support |
| `levelrail vm set` | changes a stored VM setting |
| `levelrail vm export` | a control plane backup (`internal/cpbackup`) plus app volume backups, copied to a host folder as one archive |
| `levelrail vm reset` | offers export, then deletes and recreates the VM |
| `levelrail vm upgrade` | snapshot, upgrade the guest control plane and packages, health check, roll back on failure |
| `levelrail vm uninstall` | stops and deletes the VM, the data directory, the keychain entry and any login item; `--keep-export` writes an export first |

First run in the tray app: a short screen that states what runs locally,
the memory it will use, and that nothing is reachable from other machines;
then `vm init` with progress; then the dashboard opens. The first run
downloads Lima (macOS), the guest image and container images, so it needs a
network connection and takes minutes, not seconds. A cached-image boot to
READY took 154 s on the owner's Mac (see Real boot results).

Upgrades:

- App and CLI: through the updater (decision 5) or the install script.
- Guest control plane: `vm upgrade`, pinned to the CLI version, with a
  snapshot first.
- Guest base image: a new pinned image means export, recreate, import. We
  do not upgrade Ubuntu releases in place.
- Lima pin bumps ship with the CLI and must pass the isolation check on a
  real Mac before release.

Data location: macOS `~/Library/Application Support/<brand>`, Windows
`%LOCALAPPDATA%\<brand>` (the WSL disk lives there), Linux
`$XDG_DATA_HOME/<brand>`. The name comes from `internal/brand`. Backup is
`vm export` plus the existing app volume backups; raw VM disk copies are
not a supported backup.

### 5. Release engineering

- **Channel order.** First the `vm` commands ship in the normal CLI release
  behind `APP_VM_EXPERIMENTAL=on`. Then an unsigned, experimental tray app
  installed by the install script, not a downloaded disk image, so the
  first channel needs no signing spend. Signed and notarized installers
  come only after the owner approves the spend and usage justifies it.
- **Signing budget.** An Apple Developer membership for Developer ID and
  notarization, and a Windows code signing option (a cloud signing service
  or an OV certificate). Prices and eligibility: unverified, the owner
  checks before committing. Linux packages are not platform signed;
  checksums are signed as today.
- **Updater.** The app fetches a channel manifest (version, per-OS asset
  URL, SHA-256, minimum supported version, halt flag) and its detached
  Ed25519 signature. The public key is compiled into the app. Missing or
  bad signature, hash mismatch, a redirect to an unpinned host or a
  manifest older than the installed one: stop and show an error, never
  install. The Ed25519 over SHA-256 pattern is an idea taken from
  published desktop updaters, reimplemented, no code copied. The install
  script keeps its existing cosign checksum verification. The signing key
  is a release secret held outside the repository.
- **CI.** A new `.github/workflows/desktop.yml`, separate from
  `release.yml`, builds the shell on native runners (macOS arm64, macOS
  x86_64, Windows x64, Linux x64 and arm64) on release tags and uploads to
  the experimental channel. The VM template and guest provisioning tests
  run as plain Go tests anywhere; booting a vz VM in CI is not assumed to
  work (hosted macOS runners and nested virtualization: unverified).
- **Release flow.** All of this lands on the one open `release/{version}`
  branch and ships in that release's single squash PR to `main`. The
  desktop channel manifest is published by `desktop.yml` after the release
  tag exists, never from a release branch push.

### 6. Licensing

- Levelrail is Apache-2.0. The Openship repository (oblien/openship) is
  Apache-2.0 and its desktop app (Electron) was read as a reference for
  ideas only. No file is copied. If a specific file is ever wanted, it is
  vetted first and carries its copyright notice, a change note and any
  NOTICE content.
- That reference app ships a GPL mail engine tree as a bundled resource.
  We do not copy its packaging config, and our desktop payload bundles no
  GPL component: mail and every app run as container images pulled at
  runtime, not shipped in the installer.
- OpenClaw is MIT. We ship a template that pulls the upstream image; we do
  not redistribute it.
- Lima is Apache-2.0 (verify for the pinned version). If the signed app
  bundles `limactl`, its license and notice ship with it.
- The Go webview and tray libraries are vetted in slice S6 before use.
- Streamed desktop images are pulled from upstream, not redistributed;
  their licenses are listed in the template.

### 7. Slices

Each slice owns only the listed files. Shared files are sequential.
Thresholds and limits come from env (seeding the stored VM settings where
noted).

| Slice | Model | Files | Env settings (defaults) | Real host | Review |
| --- | --- | --- | --- | --- | --- |
| S1. Lima template, cloud-init, guest isolation unit, template test | Sonnet | `internal/vmruntime/lima/` (new), `internal/vmruntime/guest/` (new) | `APP_VM_IMAGE_SHA256` (pinned in source, env override for testing only) | owner's Mac, then the Oracle arm64 VM for the guest unit | independent security review |
| S2. Host settings file and `vm init/start/stop/status/shell/set` for macOS | Sonnet | `cmd/levelrail-cli/vm.go`, `vm_darwin.go`, `vm_other.go`, `vm_test.go`, `internal/vmruntime/settings.go` | `APP_VM_EXPERIMENTAL` (off), `APP_VM_MEMORY_MIB` (3072, seeds), `APP_VM_CPUS` (2, seeds), `APP_VM_DISK_GIB` (30, seeds), `APP_VM_MEMORY_MAX_PERCENT` (50), `APP_VM_MEMORY_MIN_MIB` (2048), `APP_VM_BOOT_TIMEOUT` (180s), `APP_VM_DASHBOARD_PORT` and `APP_VM_INGRESS_PORT` (0, chosen at init) | owner's Mac | Sonnet |
| S3. Token exchange and Host and Origin checks in the control plane | Sonnet, Opus review | `internal/desktopmode/` (new), `cmd/levelrail/desktop_mode.go` | `APP_DESKTOP_MODE` (off), `APP_DESKTOP_SESSION_TTL` (12h) | owner's Mac | secret-sensitive, independent review |
| S4. `vm export/reset/upgrade/uninstall` | Sonnet | `cmd/levelrail-cli/vm_lifecycle.go`, `vm_lifecycle_test.go` | `APP_VM_UPGRADE_HEALTH_TIMEOUT` (5m) | owner's Mac | Sonnet |
| S5. Docs page and dashboard copy | Haiku draft, Sonnet review | `docs/desktop-runtime.md` (new), section in `docs/local-development.md` | none | none | Sonnet |
| S6. Tray app (unsigned, experimental), library vetting | Sonnet | `desktop/` (new module, needs owner approval for the new `go.mod`), `.github/workflows/desktop.yml` | `APP_DESKTOP_HEALTH_INTERVAL` (10s) | owner's Mac | Sonnet |
| S7. Updater and manifest signing | Sonnet | `desktop/update/`, `scripts/desktop-manifest-sign.sh` | `APP_DESKTOP_UPDATE_CHANNEL` (experimental), `APP_DESKTOP_UPDATE_CHECK_INTERVAL` (24h) | owner's Mac | secret-sensitive, independent review |
| S8. Webview window around the dashboard | Sonnet | `desktop/window/`, `web/src/features/desktop/`, `web/src/locales/en/desktop.json` | none | owner's Mac | Sonnet |
| S9. WSL2 distro, `vm_windows.go` | Sonnet | `internal/vmruntime/wsl/` (new), `cmd/levelrail-cli/vm_windows.go` | same `APP_VM_*` keys | a real Windows machine (none available today) | independent security review |
| S10. OpenClaw verified on arm64 in the VM | Sonnet | `docs/arm-compatibility.md` (only after it passes) | none | owner's Mac VM or the Oracle arm64 VM | none |
| S11. Streamed desktop community template | Haiku scaffold, Sonnet security pass | `internal/catalog/contrib/` (one new file) | none | tested through the real engine | Sonnet |

Order: S1, S2, S3 in sequence (S3 can run beside S2, no shared files),
then S4 and S5 in parallel, then S6, S7, S8. S9, S10 and S11 are
independent. No slice needs Opus to write code; Opus reviews S1 and S3
once before the beta.

## Real boot results (2026-11-01)

Measured on the owner's Mac with Lima 2.2.1, vz, aarch64, 2048 MiB, 2 CPUs
and 10 GiB, a private Lima home and the pinned image (SHA-256 matches the
pin). A fresh instance reached READY in 154 s with the image cached. A
reboot (stop and start) took 24 s. Idle with Docker up, the guest had 1950
MiB total and about 1525 MiB available.

All eight isolation checks passed on a fresh boot and again after a reboot:
`rules_loaded`, `publish_loopback`, `no_wildcard_publish`, `no_docker_tcp`,
`host_mounts`, `host_unreachable`, `lan_unreachable` and `dns_allowed`.
Guest and container traffic to the Mac loopback through 192.168.5.2 was
blocked, as was `--network host`. Internet access and container DNS worked.

The smoke run changed the design:

- A guest process reached the Mac loopback through 192.168.5.2 while the nft
  table had only a forward hook. The egress policy now needs output and input
  chains as well as forward, or guest processes and host-network containers
  are not covered.
- `_config/override.yaml` in the Lima home merged into the instance and won
  over our template (a home mount, agent forwarding and a 0.0.0.0 range
  forward all became effective). A private Lima home and `lima.CheckHome`
  are mandatory.
- The isolation self-check first passed for the wrong reason. Docker's FORWARD
  policy dropped the probes, so they failed with or without our rules. Probes
  now use a real listener and are accepted in DOCKER-USER.
- The vz network has no resolver at 192.168.5.3. The only DNS server is the
  gateway 192.168.5.2, accepted on port 53 only. The residual is that this
  is the host resolver path.

Still open:

- The deployer must deny or reject `network_mode: host`.
- The guest sshd listens on 0.0.0.0:22 and is protected only by the input
  chain.
- Share mounts, amd64 images and Lima's ssh control path with the sidecar
  were not tested.

## Alternatives rejected

- **Electron shell.** Bundles a browser engine and a Node runtime the team
  does not otherwise run, makes signing cover many helper binaries, and
  contradicts the "thin shell" direction. Its one advantage, a single
  rendering engine, is a testing cost we can carry.
- **Tauri shell.** Small and well packaged, but adds Rust to a Go and React
  team and can only run our Go code as a separate process. Revisit only if
  the Go webview libraries prove unmaintained during S6.
- **Requiring a third-party desktop container runtime on the host.** Moves
  the trust boundary and license terms to software we do not control, and
  shares one Docker daemon with everything else the user runs. `levelrail
  dev` keeps supporting it for developers, without isolation.
- **Control plane on the host, only containers in the VM.** Needs darwin and
  Windows control plane builds, a remote Docker connection into the VM,
  and splits state across host and guest.
- **A prebuilt VM disk image we host.** Faster first boot but a large
  artifact to build, sign, host and patch. Revisit after the beta.
- **Streamed full desktop as the default GUI path.** Heavier and wider
  attack surface than web UI apps, not needed for OpenClaw.
- **Lima on Linux desktops by default.** Gives a uniform boundary but adds
  a VM to the one OS that runs containers natively. Kept as an open
  question.

## Security

- The isolation claim is the product pitch for untrusted apps, so it is
  only made where a test proves it: the guest firewall baseline on macOS,
  checked on a real Mac on every Lima pin bump. Windows and Linux make no
  isolation claim until their own checks exist.
- The per-install token, the keychain entry and the updater signing key are
  secrets. S3 and S7 get an independent review, and none of these values
  appear in logs, `vm status` output or support bundles.
- Downloads at `vm init` (Lima, guest image, install script) are pinned by
  SHA-256 in source and fail closed on mismatch.
- Exceptions (shared folders, LAN or host egress, Rosetta) are explicit
  settings, off by default, shown in `vm status` and written to the audit
  log when changed through the API.

## Failure modes

| Failure | Behavior |
| --- | --- |
| No hardware virtualization, or macOS too old for vz | `vm init` refuses with the reason and points to `levelrail dev` |
| Host RAM too small, or requested memory above the cap | refuse above `APP_VM_MEMORY_MAX_PERCENT`, warn below `APP_VM_MEMORY_MIN_MIB` |
| Download fails or hash mismatch | stop, delete the partial file, no retry with a different source |
| Chosen port taken at start | pick a new free loopback port, persist it, tell the user |
| Isolation unit fails to load | Docker does not start in the guest; `vm status` shows the failed check |
| Laptop sleep and wake | VM resumes; the shell rechecks health; clock sync in the guest (need: unverified) |
| Upgrade fails health check | roll back to the snapshot, keep the old version |
| Disk full in the guest | existing disk doctor checks report it; `vm set disk` grows it on a stopped VM |
| amd64-only image on arm64 | existing `PlatformMismatch` classification and doctor message |
| Updater cannot verify | no install, error in the tray, manual download path shown |

## What cannot be verified locally

- vz on Intel Macs, and the template on macOS versions older than the
  owner's machine.
- Anything on Windows: no Windows machine is available. The WSL2 slice is
  design only until one is.
- Real memory use with OpenClaw, and with one streamed desktop on an 8 GB
  machine. Idle use at 2 GiB is measured; the other budgets above are
  planning figures.
- Gatekeeper and SmartScreen behavior for an unsigned app installed by the
  install script on current OS versions.
- Signing prices and eligibility.
- Whether hosted CI runners can boot a vz VM or nested KVM.
- Whether Lima's disk grow works for our pinned version.

## Testing plan

- S1: a Go test that renders the Lima template and asserts no mounts, no
  automatic port forwarding, only the two loopback forwards, no Docker
  socket forward and no sound device; a test that the guest unit contains
  the egress baseline rules in the right order. Real check on the owner's
  Mac: from a container, host loopback and LAN addresses are unreachable
  and the internet is reachable.
- S2 and S4: unit tests for settings precedence (stored beats env, env
  seeds once, pending restart), then one real `init, stop, start, export,
  reset, uninstall` run on the owner's Mac, leaving nothing behind.
- S3: tests for token single use, Host and Origin rejection, session TTL.
- S7: tests for every fail-closed path (bad signature, hash mismatch,
  downgrade, unpinned redirect, halt flag).
- S10: deploy the OpenClaw template through the real engine in the arm64
  guest, open the Control UI through `<app>.localhost`, compose down after.
- One focused test per slice; no guard tests that couple this feature to
  others.

## Kill switch

- `APP_VM_EXPERIMENTAL` gates the `vm` commands until the beta ends.
- The channel manifest's halt flag stops all updates and shows a message;
  removing the manifest stops new installs from the app.
- Neither deletes user data. `vm export` keeps working.

## Consequences

- Positive: a desktop user gets an isolated local runtime and the full
  catalog, including OpenClaw, with no host Docker install.
- Positive: one control plane codebase; the VM runs the same Linux build
  servers run.
- Positive: the CLI layer is useful without the desktop app.
- Negative: a support surface across three operating systems and two VM
  technologies, plus Lima and WSL behavior we do not control.
- Negative: VM settings live on the host, outside `runtimesettings`, so the
  dashboard cannot change them until the shell exposes them. This breaks
  the "dashboard, CLI and API in the same change" rule for S2 and needs an
  owner exception.
- Negative: signing costs and native CI runners once the signed channel
  starts.
- Negative: the desktop app can be mistaken for a way to host public apps.
  First run and the docs say it is not.

## Open questions (owner decisions)

1. Signing spend: approve Apple Developer ID and a Windows signing option
   now, or stay unsigned and experimental until a usage threshold (which
   one)?
2. Shell: confirm the Go shell over Tauri.
3. Windows timing: in the first public beta, or after macOS proves out?
   A Windows test machine is needed either way.
4. Is OpenClaw a flagship template shown on first run, or one entry in the
   catalog?
5. Should the desktop app also connect to a remote control plane as a
   client (ADR 057 question 3), and in which release?
6. Rosetta for amd64 images on Apple silicon: allow as an opt-in setting?
7. Linux desktops: native containers only, or an optional Lima VM on KVM
   for the same boundary as macOS? And how to apply the egress baseline
   without touching the host firewall?
8. Accept the exception to the "same change" rule for host-side VM
   settings, or require a dashboard surface before S2 ships?
9. Should `alerting.db` join the control plane backup before `vm export`
   relies on it (ADR 057 question 6)?
