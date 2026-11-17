# ADR 078: Run ingress as its own long-lived process on a single server

Status: Proposed

Date: 2026-11-16

## Context

ADR 005 embeds Caddy in the control plane process. The driver loads config with
`caddy.Load` and stops it with `caddy.Stop()` on shutdown
(`internal/ingress/driver.go`), and the boot sequence creates and stops the
driver together with everything else (`cmd/levelrail/boot_stages_services.go`).
Ingress therefore lives and dies with the control plane.

ADR 025 closed the listener gap: `install.sh` turns socket activation on for new
installs, systemd holds 80 and 443, new connections wait in the kernel backlog
during a restart, and in-flight requests get a 3s grace period
(`cmd/levelrail/ingress_hardening.go`, overridable with
`APP_INGRESS_GRACE_PERIOD`). A sealed route snapshot (`ingress-snapshot.bin`,
`internal/ingress/driver_snapshot.go`) lets the new process serve the last good
table before its first reconcile.

What remains is the gap this ADR is about. `docs/resilience.md` states it:
established keep-alive and WebSocket connections are closed after the grace
period and cut on every restart, because the connection lives in the dying
process. Held and waking requests for sleeping apps are lost too, since
`HoldTracker` is in memory and the wake hook is served by the control plane.
Every upgrade, crash restart and settings change that needs a restart drops
every open WebSocket, server-sent event stream and long poll on the box, even
on a single server with nothing else wrong.

Scope notes:

- Public DNS is not part of this. Levelrail is not a DNS server for user
  domains (`docs/dns.md`): zones and records live at the provider and outlive
  the control plane. Nothing here changes that.
- ADR 033 step 2 asks for a data plane independent of the control plane but
  only describes the multi-node step (several ingress nodes sharing certificate
  state). It does not cover splitting the process on one box.
- ADR 025 rejected "moving Caddy out into a sibling process or container" as too
  large a change for a gap of a few seconds. That reasoning held for the
  listener gap, which socket activation solved. It does not hold for
  long-lived connections: no amount of socket handoff can move an open
  connection out of a process that exits. The only way to keep them across a
  control-plane restart is to not restart the process that owns them. This ADR
  revisits that one rejected alternative; the rest of ADR 025 stands.

## Decision

Recommended: split ingress into its own long-lived process on the same host,
opt-in first, embedded mode kept as the default until proven on real hosts.

1. **Edge process.** A new subcommand of the same binary (working name
   `levelrail ingress serve`) runs Caddy and nothing else. It owns 80 and 443
   (the systemd sockets attach to its unit instead of the control plane's),
   restores the sealed snapshot at start and holds the last-known route table
   in memory.
2. **Push, not pull.** The control plane stays the only place that computes
   config. It pushes each applied config to the edge over a local admin channel
   (a unix socket in the data directory, mode 0600, owned by the service user).
   The edge validates and loads it in place (Caddy reloads without closing
   listeners) and persists it as the new snapshot.
3. **Control plane restart pauses updates.** While the control plane is down the
   edge keeps serving the last table and keeps every open connection. Route
   changes resume on the first push after the control plane is back.
4. **Edge supervision and upgrade.** The edge is its own systemd unit with its
   own restart policy. It upgrades less often than the control plane (only when
   the bundled Caddy or edge code changes) and, when it does, uses the existing
   socket handoff so new connections queue. Long-lived connections are still cut
   on an edge restart; this ADR moves that cost from every upgrade to edge
   upgrades only.
5. **Certificates.** The control plane stays the sole issuer, consistent with
   ADR 036. The edge reads certificates from the shared store.
6. **Single box first.** ADR 033 multi-node ingress builds on the same push
   protocol later; nothing here assumes more than one host.

## Alternatives considered

- **Stay embedded, improve socket activation.** Cheapest and keeps the single
  process from ADR 005. It cannot keep an open connection alive across a
  process exit; passing live TLS connections between processes would need
  handing over TLS session state, HTTP/2 stream state and Caddy internals.
  Rejected as the answer to this gap, kept as the default mode.
- **Sidecar container.** Run the edge as a Docker container next to the
  control plane. Gets isolation and a familiar upgrade path, but ties ingress
  to the Docker daemon (a daemon restart then also cuts ingress), adds image
  pulls to install and upgrade, and complicates socket activation and host
  networking. Rejected for single box; may suit Compose installs later.
- **External proxy.** Users run their own reverse proxy in front and Levelrail
  writes its config. Moves the problem to the user, loses on-demand TLS, hold
  pages and the shared certificate store, and every proxy needs its own
  adapter. Rejected as a product default; nothing stops users doing it today.
- **SO_REUSEPORT overlap or longer drain.** Already rejected in ADR 025; still
  cannot move established connections.

## Consequences

- **Upgrade and rollback.** A control-plane upgrade no longer touches the edge.
  A rollback of the control plane is equally safe. Version skew becomes real:
  the edge must report its build and loaded module list, and the control plane
  must refuse to push a config the edge cannot load, leaving the edge on its
  last good table. Rolling back the edge falls back to the snapshot rules
  already in `docs/resilience.md` (a snapshot Caddy rejects is ignored).
- **Supervision.** Two units instead of one. Boot order: edge first, so the
  sockets have an owner before the control plane starts. A crash loop in the
  edge is a full outage, so its start limits need their own settings.
- **Consistency and staleness.** The edge can be stale for as long as the
  control plane is down. A deleted app keeps its route, and a new app has none,
  until the next push. The staleness window should be visible (last push time,
  age) in doctor and on the System status page, with a warning threshold read
  from a setting rather than a constant.
- **Security boundary.** The edge holds TLS private keys, DNS-01 tokens in
  config and basic auth hashes, the same as today, but in a smaller process
  with no API, no database writes and no Docker socket. It needs no extra
  privilege: the ports come from systemd. The admin channel is local only and
  never bound to TCP. How the edge opens the sealed snapshot (master key or a
  derived edge key) is open.
- **Footprint.** One more resident process. The edge should stay within a
  small memory budget so Levelrail stays lightweight; the budget is a setting
  checked by doctor, not a hardcoded number, and is measured before any claim.
- **Observability.** Access logs and request metrics move to the edge process;
  the control plane must pull or receive them, and the System page needs an
  edge row (up, version, last push, snapshot age).
- **Migration.** Existing installs switch with `install.sh upgrade` and a mode
  setting; switching stops service once. Hosts without systemd (Compose, dev)
  keep embedded mode; there is nothing to hold the sockets.
- **Sleeping apps while the control plane is down.** Open question. Options:
  (a) the edge serves a static 503 "starting" page and wakes nothing until the
  control plane returns; (b) the edge queues the wake and forwards it once the
  control plane socket answers; (c) the edge starts the container itself, which
  needs Docker access in the edge and widens its privilege; (d) apps with wake
  on request are kept awake while the edge is in this mode.

## Open questions

1. Behaviour of wake-on-request apps while the control plane is down (options
   above).
2. Cert store access: the edge reads the SQLite-backed store through the
   existing lock (`AcquireCertStorageLock`), or receives cert bundles over the
   push channel as ADR 036 describes for remote nodes.
3. Which key the edge uses for the sealed snapshot.
4. Where the hold window state lives (edge, control plane or both).
5. Whether the edge carries the layer4 routes (`internal/ingress/layer4.go`) or
   only HTTP.
6. Whether Compose installs get a container edge or stay embedded.
7. Whether split mode becomes the default for new systemd installs, and what
   evidence is required first.

## Testing plan

- Unit: push protocol round trip, version handshake refusal, edge restores the
  snapshot with no control plane running, fake loader rejects a config and the
  edge keeps the last good table.
- Two-process test on one host: kill and restart the control plane while a
  WebSocket client stays connected; count closes (expected zero).
- Real host (Linux, systemd, real Docker and Caddy): WebSocket soak across
  several control-plane upgrades with zero closes; edge restart counted
  separately; host reboot ordering; switch from embedded and back; uninstall.

## What cannot be verified locally

Socket handoff under real systemd, boot ordering, memory footprint on a small
VPS, certificate renewal while the control plane is down, and behaviour under
real load. The existing route snapshot itself is still unit tested only
(`docs/resilience.md`, "Not proven live").

## Rollout (small slices, no dates)

Each slice is opt-in behind `ingress.process_mode` (`embedded` default,
`separate`), a runtime setting seeded by `APP_INGRESS_PROCESS_MODE`. New
thresholds (push timeout, staleness warning, memory budget) are runtime
settings seeded from `APP_INGRESS_*` env, never constants.

1. Edge subcommand that loads the snapshot and serves. Owns
   `cmd/levelrail/ingress_serve.go`, `internal/ingress/edge.go`.
2. Local admin channel and push client. Owns `internal/ingress/edge_admin.go`,
   `internal/ingress/driver_remote.go`.
3. Driver switch by mode. Shared, one lane: `internal/ingress/driver.go`,
   `cmd/levelrail/boot_stages_services.go`.
4. Version handshake and last-good guard. Owns
   `internal/ingress/edge_version.go`.
5. Cert store access per open question 2. Shared: `internal/ingress/certstorage.go`.
6. Hold and wake behaviour per open question 1. Shared: `internal/ingress/hold.go`.
7. `install.sh`: edge unit, sockets attached to it, migration and uninstall.
   One lane only.
8. Doctor row, System status edge row, `docs/resilience.md` update, and a
   WebSocket soak in `levelrail-cli bench`.
9. Real-host proof before any default change.

Slices 1, 3 and 7 are each likely too large for one pass and should be split
when started.
