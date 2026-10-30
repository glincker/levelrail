# ADR 057: A desktop app that runs the control plane only while it is open

Status: Proposed

Date: 2026-10-29

## Context

The question: can a solo operator manage their servers from a desktop app
on macOS, Windows or Linux, with the control plane running on their own
machine only while the app is open, so they never run an always-on control
plane? This ADR reads the code on `release/v0.8.11` to see what exists,
what is missing, and whether it is worth doing now.

What exists, verified by reading the code and docs:

1. **Local mode already runs a full control plane on a laptop.**
   `internal/localmode/localmode.go` defines `APP_LOCAL_MODE` (a
   loopback-only instance) and `APP_RESTART_SUPERVISED` (a parent relaunches
   it after a reset exit), plus `ValidateListenAddr`, which rejects any
   listen address reachable beyond the machine. `ports.go` picks free
   loopback ports (`ChoosePorts`, `LoopbackFree`), `state.go` keeps a
   generated admin in `local-admin.json` (mode 0600) and a pid and URL
   state file. `cmd/levelrail-cli/dev.go` is the supervisor: it detects a
   Docker socket (`localmode.DetectDocker`), sets every `APP_*_ADDR` to
   loopback, sets `APP_ALLOW_INSECURE_LOGIN=true`, and runs the
   `levelrail` binary as a child (`devSupervisor.runOnce`).
   `cmd/levelrail/local_mode.go` (`applyLocalMode`) refuses non-loopback
   listeners on the control plane side. `docs/local-development.md` says
   local mode needs Docker running locally, serves plain HTTP, and "is not a
   way to host anything for other people".
2. **One Go binary with the dashboard embedded, and a pure Go SQLite.** The
   release workflow builds the control plane with `-tags embedweb`
   (`.github/workflows/release.yml:93`), and `internal/api/devmode_release.go`
   carries the build tag. `go.mod` uses `modernc.org/sqlite`, so the store
   needs no CGO.
3. **The control plane binary is only released for Linux.** `release.yml`
   builds `levelrail` for `linux/{amd64,arm64}` only (`:60` to `:104`). The
   CLI is built for `{linux,darwin,windows} x {amd64,arm64}` (`:110` to
   `:151`), and CI cross-compiles the CLI for Windows and darwin
   (`.github/workflows/ci.yml:235`). `docs/local-development.md` says local
   mode was checked on macOS on Apple silicon, so the control plane builds
   for darwin, but users must build or fetch that binary themselves.
4. **The control plane does not build for Windows today, by inspection.**
   `syscall.Statfs` is used without a build tag in `internal/api/status.go`,
   `internal/api/doctor.go` and `cmd/levelrail/boot_profile.go` (and
   `syscall`/`unix` calls appear in several more files under `internal/api`).
   `Statfs` does not exist in Go's `syscall` package on Windows. I did not
   compile it; this is from reading.
5. **Agents dial the control plane, never the other way.** `internal/agent/client.go`
   is the agent side (`DialEnroll`, then a persistent `RunSession` stream with
   heartbeats and HTTP/2 pings). `docs/multi-node.md` states "The agent dials
   out to the control plane, and the control plane never initiates a
   connection", and that `APP_AGENT_ADVERTISE_HOST` must match what agents
   dial or the session fails TLS hostname verification. A laptop behind NAT
   is not reachable by a remote agent.
6. **SSH paths exist, but only outbound commands.** `internal/sshprovision`
   adopts an existing machine by dialing SSH, detecting OS and arch, ensuring
   Docker and systemd, and installing the agent, with host key pinning
   (`hostkey.go`). `internal/dockerssh` opens the remote Docker socket over an
   SSH session (`tunnel.go`, read allowlist in `allowlist.go`) for the read
   only import in `docs/migrate-over-ssh.md`. A grep for
   `RequestRemoteForward`, `tcpip-forward` or `client.Listen(` in `internal`
   and `cmd` finds nothing: there is no SSH remote port forward today.
7. **Ingress runs inside the control plane process.** `docs/multi-node.md`
   (around line 669): "The embedded Caddy ingress runs in the control plane's
   own process. A worker node has no public listener." Ingress reaches a
   worker's app through the WireGuard mesh, and the control plane is the mesh
   hub. While a node's agent is down its containers keep running, but with
   the mesh in the agent "the ingress path from the control plane to the
   node's apps is down too". This is the most important fact for this ADR:
   **if the control plane lives on a laptop, public traffic to apps on the
   servers stops when the laptop closes**, and while it is open the traffic
   would hairpin through the laptop. ADR 005 locks ingress into the
   control plane, and ADR 025 only closes the restart gap of a few
   seconds with systemd socket activation; it does not keep serving while
   the control plane is away for hours.
8. **Control plane backup and restore exist.** `internal/cpbackup` has
   snapshots, encrypted off-box backups (`remote.go`), a continuous replica
   (`replica.go`, `ReplicaInterval` from env), `Restore` (`restore.go:143`)
   and drills. `docs/control-plane-backup.md` notes `alerting.db` is not in
   the backup and that a restore needs the same master key.
9. **The master key can already be wrapped by an external key service.**
   `internal/secrets/keywrap.go` defines `KeyWrapper` (seal and open the
   serialized master key) and `WrappedPrefix`; the one implementation is
   `VaultTransit` (`transit.go`). The key itself comes from `APP_MASTER_KEY`
   or `APP_MASTER_KEY_FILE`.
10. **CLI pairing exists.** `internal/api/cli_pairing.go` serves
    `POST /api/v1/auth/cli-pairing` with an eight character code
    (`XXXX-XXXX`) and a TTL from env (`cliPairingTTL`), used by the Connect
    your CLI flow (`web/src/locales/en/settings.json`, `signIn.json`).
11. **Self upgrade is server shaped.** `internal/selfupgrade` verifies
    releases with cosign and a trust root (`sigstore.go`, `trustroot.go`),
    takes restore points, guards against downgrades (`guard.go`), and its
    host operations are "HTTPS, cosign, systemd, sqlite, filesystem"
    (`apply.go:32`). `install-cli.sh` and `install-cli.ps1` verify
    `checksums.txt` with cosign keyless signatures. Nothing signs with an
    Apple or Windows code signing identity today.

## Decision (recommended)

**Do not build the desktop app now.** Revisit it after the CLI first paths
(the install scripts, a package manager tap and an npm wrapper) show real
usage, and only after a server side ingress exists. The reason is fact 7:
the product promise "the app runs the control plane only while open, and
your apps keep running" is only half true with today's architecture.
Containers keep running, but public traffic, certificate renewal, alerts,
scheduled work, the status page sampler and deploy webhooks all depend on
the control plane being up and reachable. Shipping a desktop app that
quietly takes a user's sites offline when their laptop sleeps is worse than
not shipping one.

What to do now instead, in order, all small and useful on their own:

1. Release the `levelrail` control plane for darwin in the release matrix,
   so `levelrail-cli dev` works from the install script without building it.
2. Make the recommended solo path "install the control plane on your first
   server" (it is already a single binary with SQLite), and make the Connect
   your CLI pairing flow the way a laptop talks to it.
3. Add an OS keychain `KeyWrapper` (useful for local mode today).

If and when the desktop app goes ahead, this is the shape it should take:

- **Shell:** a thin native shell around the system webview (WebKit on
  macOS, WebView2 on Windows, WebKitGTK on Linux), written in Go, that
  starts the existing `levelrail` binary as a sidecar. Not a larger runtime
  that bundles its own browser engine and a JavaScript server runtime.
- **Process model:** the shell starts the sidecar on a random loopback port
  with a per launch token, never on a non-loopback address.
- **Connection direction:** the laptop dials each server over SSH and opens
  an SSH remote port forward, so the agent on the server dials
  `127.0.0.1:<port>` on its own host and that traffic rides the SSH session
  back to the laptop's loopback agent listener. The control plane still
  never accepts an inbound connection from the internet, and it works when
  the laptop is behind NAT.
- **Ingress:** a prerequisite, separate ADR for a server side edge (the
  node serves its own apps, with routes and certificates pushed by the
  control plane and kept when it is away). Until that exists the desktop app
  is labeled experimental and is honest that apps are not publicly routed
  while it is closed.
- **Team use is a non goal.** One operator, one machine, one admin.

### 1. Shell choice

Criteria, with the team being Go and React:

| Criterion | Thin system webview shell plus Go sidecar | Larger runtime bundling a browser engine |
| --- | --- | --- |
| Download size | shell in the low megabytes plus the sidecar (sidecar size not measured) | adds roughly a hundred megabytes or more before the sidecar |
| Memory | one system webview process | a full browser engine per app |
| Signing and notarization | sign one shell and one sidecar binary | sign the runtime, its helpers and the sidecar |
| Auto update | our own updater, reusing cosign verification | runtime updater, but a second trust chain to maintain |
| CI build | needs native runners (CGO for WebKit and WebKitGTK) for the shell only; sidecar stays pure Go and cross compiles | needs native runners too, plus a JavaScript toolchain for the shell |
| Team fit | Go and React only | adds a JavaScript server runtime to own |
| Rendering consistency | three webview engines to test the dashboard on | one engine everywhere |

The thin shell wins on every criterion except rendering consistency, and the
dashboard already targets current browsers, so three engines is a testing
cost, not a blocker. The shell is only a window, a tray icon, a supervisor
and an updater. All product logic stays in the sidecar, which is the same
binary servers run. That keeps one UI (the existing dashboard), not two.

### 2. Process model

- The shell picks a free loopback port with the same logic as
  `localmode.ChoosePorts`, and generates a 32 byte random launch token.
- It starts the sidecar with `APP_DESKTOP_MODE=on`, the data dir, the port
  and the token passed in the child environment (not argv, which other local
  users can read in the process list). The sidecar binds `127.0.0.1` only;
  `localmode.ValidateListenAddr` already refuses anything else.
- The webview opens `http://127.0.0.1:<port>/desktop/bootstrap` with the
  token in a header the shell injects. The sidecar exchanges it once for an
  HttpOnly, SameSite=Strict session cookie and forgets it. Every request
  checks the `Host` header is the loopback address and port (DNS rebinding)
  and the `Origin` on state changing requests.
- The generated admin file and `APP_ALLOW_INSECURE_LOGIN` from local mode
  are not used: the launch token replaces the password.
- The agent gRPC listener is also loopback only. It is reached from servers
  only through SSH remote forwards.
- The shell supervises the sidecar the way `devSupervisor` does, including
  the reset relaunch (`APP_RESTART_SUPERVISED`). Quitting sends a graceful
  stop and waits `APP_DESKTOP_SHUTDOWN_GRACE`.
- The sidecar runs with no local Docker node. Desktop mode must start
  without a Docker socket, which local mode does not support today
  (`dev.go` stops if `DetectDocker` fails).

### 3. Reaching servers

| Path | Works behind NAT | Notes |
| --- | --- | --- |
| SSH provisioning (`internal/sshprovision`) | yes, outbound SSH | already exists; install the agent with `APP_CONTROL_PLANE_ADDR=127.0.0.1:<forwarded port>` |
| Agent over SSH remote forward (new) | yes | per server forward on a port the control plane picks; `APP_AGENT_ADVERTISE_HOST` stays `127.0.0.1`, which is the default and matches the tunnel |
| Agent dials a public relay (new) | yes | needs a hosted relay we operate, which reintroduces an always-on service; rejected for now |
| Agent dials the laptop directly | no | needs an inbound port on the laptop |
| Docker over SSH only, no agent (`internal/dockerssh`) | yes | read only today by design; turning it into a write path would fork the node model |

Recommended: SSH remote forward. The server must accept SSH from the
laptop, which SSH provisioning already requires. The mesh stays off in
desktop mode, because the control plane is the WireGuard hub and a laptop
has no stable public endpoint, so cross node features that need the mesh
are unavailable.

### 4. Where data lives

- Data dir per OS: `~/Library/Application Support/<brand>` on macOS,
  `%APPDATA%\<brand>` on Windows, `$XDG_DATA_HOME/<brand>` on Linux. The
  name comes from `internal/brand`.
- The master key is stored wrapped with a new `KeyWrapper` backed by the
  macOS Keychain, Windows DPAPI with the Credential Manager, or the Linux
  Secret Service. If the Linux Secret Service is missing, the app refuses to
  start and explains, rather than writing a plain key file.
- SSH keys for servers are stored encrypted with the existing secrets
  envelope, never in plain files.
- Snapshots from `internal/cpbackup` run on app start and before each
  upgrade, since the scheduled loop only runs while open.

### 5. When the app closes

Apps keep running on servers, because each node's Docker owns its
containers (`docs/multi-node.md`). These stop while the app is closed:
public ingress and certificate renewal (fact 7, until a server side edge
exists), alert evaluation and delivery, scheduled tasks, the status page
sampler, cpbackup schedules and the replica, autoscale and reconcile loops,
and deploy webhooks. Webhooks from git providers also fail while the app is
open, because a laptop has no public URL.

How to communicate it:

- First run shows a plain list of what does and does not run while closed,
  and asks the user to confirm.
- The window and tray show a "control plane is on this computer" badge.
- Quitting shows the same list once per session, with "Move the control
  plane to a server" as the primary action.
- Status, alert and webhook settings pages show an inline notice in desktop
  mode.

One click move: take a cpbackup snapshot (plus `alerting.db`, which the
backup does not include today), copy it over SSH to a chosen server,
install the control plane there, restore with the master key unwrapped from
the keychain and sent over the SSH session, then re-point agents. An agent
on that same server can keep `127.0.0.1:<agent port>`. Agents on other
servers need a new address and possibly a new server certificate name; how
the existing CA pinning behaves across that move is not verified. After a
verified health check the desktop app switches to client mode: a window
onto the remote control plane, paired through the Connect your CLI flow.

### 6. Updates and signing

- One bundle carries the shell and the sidecar at the same version, so the
  sidecar never meets a database from a newer release (`selfupgrade/guard.go`
  logic still runs) and a restore point is taken before migrations.
- The updater fetches a manifest, verifies the cosign bundle on
  `checksums.txt` with the same identity and issuer as `install-cli.ps1`,
  then lets the OS verify the platform signature.
- macOS: Developer ID signing and notarization need the Apple Developer
  Program (a yearly fee). Windows: an OV or EV code signing certificate or a
  cloud signing service (a yearly fee, plus SmartScreen reputation that takes
  time to build). Linux: no platform signing; cosign signatures and a signed
  apt repository if we ship one. Current prices are not verified here.
- Packaging: dmg (universal or per arch) on macOS, msi on Windows (exe
  installer optional), AppImage and deb on Linux.

## Non goals

- Teams, shared access, SSO, or more than one operator per desktop app.
- Exposing the desktop control plane beyond loopback.
- A second dashboard. The desktop app renders the existing `web/src`
  dashboard.
- Windows or macOS nodes. Running the control plane on a laptop does not
  change the non goal of Linux only nodes; the desktop sidecar runs no
  local node at all.

## Alternatives rejected

- **Build it now as described.** Rejected for now: the ingress gap makes
  the main promise misleading, Windows does not compile, and signing costs
  and support land before we know solo operators want it.
- **Do nothing and improve `levelrail-cli up` or `dev`.** Kept as the
  near term path, not rejected. It reaches the same users with no signing
  cost and no new packaging, but leaves the "no always-on control plane"
  wish unmet.
- **A browser based local mode** (local mode plus SSH remote forward, opened
  in the user's own browser). Cheaper than a shell and a good first step if
  the forward is built, but has no tray, no keychain integration out of the
  box and no clear "quit" moment to warn about what stops.
- **A hosted control plane we run.** Solves the always-on problem fully but
  is a hosted product with its own security, cost and support model, and
  the product strategy rules out a managed offering until the self-hosted
  version has real users.
- **A bundled browser engine runtime for the shell.** Rejected in section 1
  on size, memory, signing surface and team fit.
- **A public relay for agents.** Reintroduces an always-on service we would
  operate.

## Security

- Loopback only listeners, refused otherwise by `ValidateListenAddr`.
- Any local process can reach a loopback port, so every request needs the
  session from the one time launch token, with Host and Origin checks.
- The launch token is never logged and never written to disk.
- The master key stays wrapped by the OS keychain, and leaves the machine
  only during an explicit move to a server, over the SSH session.
- SSH host keys are pinned with the existing `sshprovision/hostkey.go`
  flow, never trust on first use silently.
- SSH remote forwards bind to `127.0.0.1` on the server, never
  `0.0.0.0`, so other users on the server reach nothing beyond what an
  agent with a valid client certificate could; mTLS still runs inside the
  tunnel.
- A stolen laptop holds SSH keys to every server. The keychain protects the
  master key at rest, but disk encryption is the user's responsibility; the
  first run should say so.
- Updates are verified twice (cosign and OS signature) before the shell
  replaces itself.

## Failure modes

| Failure | Effect | Handling |
| --- | --- | --- |
| Laptop sleeps or loses network | SSH forwards drop, nodes go `offline`, changes return 409 `node_offline` | existing offline semantics; reconnect with backoff up to `APP_DESKTOP_TUNNEL_RECONNECT_MAX` |
| Sidecar crashes | window shows a stopped state | shell restarts it, after `APP_DESKTOP_SIDECAR_RESTART_LIMIT` attempts shows logs |
| Sidecar does not become healthy | blank window | shell waits `APP_DESKTOP_SIDECAR_READY_TIMEOUT`, then shows the log tail |
| Port taken between pick and bind | sidecar exits | shell picks a new port and retries |
| Keychain locked or missing | master key unavailable | refuse to start with a clear message, never fall back to plain text |
| Update interrupted | half replaced bundle | OS installer is atomic on macOS and Windows; AppImage keeps the old file until verified |
| Move to server fails half way | two control planes or none | desktop stays authoritative until the server passes health and the user confirms; agents are re-pointed last |
| Two copies of the app open | two sidecars on one data dir | single instance lock in the data dir |

## What cannot be verified locally

- Notarization, Gatekeeper and SmartScreen behavior, and their real costs.
- WebView2, WebKit and WebKitGTK rendering of the full dashboard.
- Control plane size and memory on a laptop: not measured for this ADR.
- That the control plane starts and runs well without a local Docker node.
- Long lived SSH remote forwards across sleep, Wi-Fi changes and captive
  networks, and server side `AllowTcpForwarding` settings (some hosts
  disable it).
- How CA pinning and `APP_AGENT_ADVERTISE_HOST` behave when agents are
  re-pointed during a move.
- Windows compile status: read from the source, not built.
- Linux Secret Service availability across desktop environments.

## Testing plan

- Unit: launch token exchange (one use, wrong token, missing Host, foreign
  Origin), loopback refusal, port retry, keychain wrapper with a fake store.
- Unit: SSH remote forward against an in-process SSH server, including a
  dropped connection and a server that refuses forwarding.
- Integration: sidecar in desktop mode with no Docker socket, enroll one
  agent through a forward, deploy one app, close, reopen, verify the node
  reconnects and the app condition clears.
- Manual per release: install from the signed artifact on a clean machine
  for each OS, update from the previous version, run the move to server
  drill.
- One focused test per slice; no broad guard tests that couple features.

## Phases

1. Now: darwin control plane in the release matrix, OS keychain
   `KeyWrapper`, Connect your CLI as the laptop path. No desktop app.
2. Prerequisite ADR: server side edge ingress that survives the control
   plane being away.
3. Desktop spike (first slice below), behind an experimental flag, not
   announced.
4. macOS beta with the close semantics UI and the move to server action.
5. Windows (fix the Statfs build first), then Linux.
6. Auto update and general availability.

## First slice (under 1 week, if approved)

A signed and notarized macOS build that opens the existing dashboard on a
loopback sidecar control plane and connects to one server over SSH.

- Shell: window, sidecar supervision, launch token, quit with a stop list.
- Sidecar: `APP_DESKTOP_MODE`, token bootstrap, start without local Docker.
- One SSH remote forward and agent install through `sshprovision`.
- Exit criterion: the server shows `online`, an app deploys to it, closing
  the app turns the node `offline` without stopping the container, and
  reopening brings it back. Public routing of that app is out of scope for
  this slice (fact 7).

## Slices and file ownership

Each slice owns only the files listed. A new Go module for the shell and any
`go.mod` change need owner approval first.

| Slice | Files | Env settings (defaults) |
| --- | --- | --- |
| A. darwin control plane release | `.github/workflows/release.yml` | none |
| B. OS keychain wrapper | `internal/secrets/oskeychain.go`, `oskeychain_darwin.go`, `oskeychain_windows.go`, `oskeychain_linux.go`, tests | `APP_MASTER_KEY_WRAPPER=oskeychain` |
| C. Desktop mode in the sidecar | `cmd/levelrail/desktop_mode.go`, `internal/desktopmode/` (token, bootstrap handler, host checks) | `APP_DESKTOP_MODE` (off), `APP_DESKTOP_LAUNCH_TOKEN` (set by shell), `APP_DESKTOP_SESSION_TTL` (12h) |
| D. SSH remote forward | `internal/sshtunnel/` (new), wiring in `internal/sshprovision/sshprovision.go` | `APP_DESKTOP_TUNNEL_KEEPALIVE` (15s), `APP_DESKTOP_TUNNEL_RECONNECT_MAX` (60s), `APP_DESKTOP_AGENT_PORT_RANGE` (40000-40999) |
| E. Shell | `desktop/` (new, its own module), `.github/workflows/desktop.yml` | `APP_DESKTOP_SIDECAR_READY_TIMEOUT` (30s), `APP_DESKTOP_SIDECAR_RESTART_LIMIT` (3), `APP_DESKTOP_SHUTDOWN_GRACE` (10s) |
| F. Close semantics UI | `web/src/features/desktop/`, `web/src/locales/en/desktop.json` | none |
| G. Move to server | `internal/desktopmode/handoff.go`, CLI verb in `cmd/levelrail-cli/` | `APP_DESKTOP_HANDOFF_HEALTH_TIMEOUT` (5m) |
| H. Windows build | `internal/api/status.go`, `internal/api/doctor.go`, `cmd/levelrail/boot_profile.go` (split Statfs into build tagged files) | none |
| I. Updater | `desktop/update/` | `APP_DESKTOP_UPDATE_CHANNEL` (stable), `APP_DESKTOP_UPDATE_CHECK_INTERVAL` (24h) |

Slices A and B are useful without the desktop app. C and D can land behind
the flag in parallel because they share no files. E depends on C. H is
independent and also unblocks a Windows local mode.

## Consequences

- Positive: a solo operator could manage servers without paying for or
  running an always-on control plane, from a familiar app with a tray icon.
- Positive: slices A, B, D and H improve local mode and SSH paths even if
  the desktop app never ships.
- Negative: a new support surface on three operating systems, with the
  most confusing class of bug ("my site went down when I closed my laptop").
- Negative: yearly signing costs and native CI runners, which cost more
  than Linux runners.
- Negative: two ways to run the control plane (server and desktop) with
  different guarantees, even though there is one UI.
- Negative: features that need the mesh or a public URL (cross node
  ingress, webhooks, status page as seen from outside) are reduced in
  desktop mode.

## Open questions

1. What usage signal from the CLI first paths should trigger revisiting
   this (install counts, issues asking for it), and who owns that call?
2. Is a server side edge ingress acceptable architecturally (it needs a new
   ADR against ADR 005, and fits the "data plane independent of the control
   plane" direction), or does it break features that assume ingress lives in
   the control plane (`proxyroutes` and `proxyinstall` may be a starting point; not
   checked in depth)?
3. Should the desktop app ship first only as a client for a remote control
   plane (pairing through Connect your CLI), which avoids the close
   semantics entirely?
4. Do we accept the yearly Apple and Windows signing costs now, or keep
   the desktop app unsigned and experimental until usage justifies them?
5. How do agents on other servers get re-pointed after a move without a
   new enrollment, given CA pinning and the advertise host check?
6. Should `alerting.db` join the control plane backup first, since the move
   to server depends on it?
